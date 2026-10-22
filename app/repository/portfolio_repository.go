package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"api-students/app/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrPortfolioNotFound = errors.New("portfolio tidak ditemukan")
	ErrProjectNotFound   = errors.New("proyek tidak ditemukan")
	ErrProjectLimit      = errors.New("jumlah proyek sudah mencapai batas maksimal 20")
)

const MaxProjectsPerPortfolio = 20

type PortfolioRepository interface {
	EnsureIndexes(ctx context.Context) error
	FindByStudentID(ctx context.Context, studentID int) (model.Portfolio, error)
	Upsert(ctx context.Context, p model.Portfolio) (model.Portfolio, error)
	AddProject(ctx context.Context, studentID int, project model.Project) (model.Portfolio, error)
	DeleteProject(ctx context.Context, studentID int, projectID string) error
	GetTechnologyStats(ctx context.Context, limit int) ([]model.TechnologyStat, error)
}

type portfolioDocument struct {
	ID           bson.ObjectID         `bson:"_id,omitempty"`
	StudentID    int                   `bson:"student_id"`
	Headline     string                `bson:"headline"`
	Bio          string                `bson:"bio"`
	Projects     []projectDocument     `bson:"projects"`
	Certificates []certificateDocument `bson:"certificates"`
	CreatedAt    time.Time             `bson:"created_at"`
	UpdatedAt    time.Time             `bson:"updated_at"`
}

type projectDocument struct {
	ID           bson.ObjectID `bson:"_id"`
	Title        string        `bson:"title"`
	Technologies []string      `bson:"technologies"`
	Link         string        `bson:"link"`
	Year         int           `bson:"year"`
}

type certificateDocument struct {
	ID     bson.ObjectID `bson:"_id"`
	Name   string        `bson:"name"`
	Issuer string        `bson:"issuer"`
	Date   string        `bson:"date"`
}

func (d portfolioDocument) toModel() model.Portfolio {
	projects := make([]model.Project, 0, len(d.Projects))
	for _, p := range d.Projects {
		techs := p.Technologies
		if techs == nil {
			techs = []string{}
		}
		projects = append(projects, model.Project{
			ID:           p.ID.Hex(),
			Title:        p.Title,
			Technologies: techs,
			Link:         p.Link,
			Year:         p.Year,
		})
	}

	certificates := make([]model.Certificate, 0, len(d.Certificates))
	for _, c := range d.Certificates {
		certificates = append(certificates, model.Certificate{
			ID:     c.ID.Hex(),
			Name:   c.Name,
			Issuer: c.Issuer,
			Date:   c.Date,
		})
	}

	return model.Portfolio{
		ID:           d.ID.Hex(),
		StudentID:    d.StudentID,
		Headline:     d.Headline,
		Bio:          d.Bio,
		Projects:     projects,
		Certificates: certificates,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

type portfolioMongoRepository struct {
	coll *mongo.Collection
}

func NewPortfolioRepository(db *mongo.Database) PortfolioRepository {
	collName := "student_portfolios"
	if env := os.Getenv("MONGO_PORTFOLIO_COLLECTION"); env != "" {
		collName = env
	}
	return &portfolioMongoRepository{coll: db.Collection(collName)}
}

// EnsureIndexes membuat unique index pada student_id dan multikey index pada projects.technologies.
func (r *portfolioMongoRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// Setiap mahasiswa hanya memiliki tepat SATU dokumen portofolio
			Keys:    bson.D{{Key: "student_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("student_unique"),
		},
		{
			// Multikey index untuk mempercepat filter dan agregasi teknologi
			Keys:    bson.D{{Key: "projects.technologies", Value: 1}},
			Options: options.Index().SetName("projects_technologies"),
		},
	})
	if err != nil {
		return fmt.Errorf("membuat index student_portfolios: %w", err)
	}
	return nil
}

func (r *portfolioMongoRepository) FindByStudentID(ctx context.Context, studentID int) (model.Portfolio, error) {
	var doc portfolioDocument
	err := r.coll.FindOne(ctx, bson.M{"student_id": studentID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Portfolio{}, ErrPortfolioNotFound
		}
		return model.Portfolio{}, fmt.Errorf("mengambil portfolio: %w", err)
	}
	return doc.toModel(), nil
}

func (r *portfolioMongoRepository) Upsert(ctx context.Context, p model.Portfolio) (model.Portfolio, error) {
	projDocs := make([]projectDocument, 0, len(p.Projects))
	for _, pr := range p.Projects {
		pID := bson.NewObjectID()
		if pr.ID != "" {
			if parsed, err := bson.ObjectIDFromHex(pr.ID); err == nil {
				pID = parsed
			}
		}
		techs := pr.Technologies
		if techs == nil {
			techs = []string{}
		}
		projDocs = append(projDocs, projectDocument{
			ID:           pID,
			Title:        pr.Title,
			Technologies: techs,
			Link:         pr.Link,
			Year:         pr.Year,
		})
	}

	certDocs := make([]certificateDocument, 0, len(p.Certificates))
	for _, cr := range p.Certificates {
		cID := bson.NewObjectID()
		if cr.ID != "" {
			if parsed, err := bson.ObjectIDFromHex(cr.ID); err == nil {
				cID = parsed
			}
		}
		certDocs = append(certDocs, certificateDocument{
			ID:     cID,
			Name:   cr.Name,
			Issuer: cr.Issuer,
			Date:   cr.Date,
		})
	}

	currentTime := now()
	update := bson.M{
		"$set": bson.M{
			"headline":     p.Headline,
			"bio":          p.Bio,
			"projects":     projDocs,
			"certificates": certDocs,
			"updated_at":   currentTime,
		},
		"$setOnInsert": bson.M{
			"student_id": p.StudentID,
			"created_at": currentTime,
		},
	}

	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var doc portfolioDocument
	err := r.coll.FindOneAndUpdate(ctx, bson.M{"student_id": p.StudentID}, update, opts).Decode(&doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return model.Portfolio{}, ErrDuplicate
		}
		return model.Portfolio{}, fmt.Errorf("menyimpan portfolio: %w", err)
	}

	return doc.toModel(), nil
}

// AddProject menambahkan satu proyek secara atomik dengan pola conditional update:
// {"projects.19": {"$exists": false}} bersama operasi $push.
func (r *portfolioMongoRepository) AddProject(ctx context.Context, studentID int, project model.Project) (model.Portfolio, error) {
	doc := projectDocument{
		ID:           bson.NewObjectID(),
		Title:        project.Title,
		Technologies: project.Technologies,
		Link:         project.Link,
		Year:         project.Year,
	}
	if doc.Technologies == nil {
		doc.Technologies = []string{}
	}

	limitField := "projects." + strconv.Itoa(MaxProjectsPerPortfolio-1) // projects.19

	var updated portfolioDocument
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"student_id": studentID, limitField: bson.M{"$exists": false}},
		bson.M{
			"$push": bson.M{"projects": doc},
			"$set":  bson.M{"updated_at": now()},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == nil {
		return updated.toModel(), nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.Portfolio{}, fmt.Errorf("menambah proyek: %w", err)
	}

	count, countErr := r.coll.CountDocuments(ctx, bson.M{"student_id": studentID})
	if countErr != nil {
		return model.Portfolio{}, fmt.Errorf("memeriksa portfolio: %w", countErr)
	}
	if count == 0 {
		return model.Portfolio{}, ErrPortfolioNotFound
	}
	return model.Portfolio{}, ErrProjectLimit
}

// DeleteProject menghapus satu proyek spesifik menggunakan operator $pull.
func (r *portfolioMongoRepository) DeleteProject(ctx context.Context, studentID int, projectID string) error {
	pOID, err := bson.ObjectIDFromHex(projectID)
	if err != nil {
		return ErrInvalidID
	}

	res, err := r.coll.UpdateOne(ctx,
		bson.M{"student_id": studentID, "projects._id": pOID},
		bson.M{
			"$pull": bson.M{"projects": bson.M{"_id": pOID}},
			"$set":  bson.M{"updated_at": now()},
		},
	)
	if err != nil {
		return fmt.Errorf("menghapus proyek: %w", err)
	}

	if res.MatchedCount == 0 {
		count, countErr := r.coll.CountDocuments(ctx, bson.M{"student_id": studentID})
		if countErr != nil {
			return fmt.Errorf("memeriksa portfolio: %w", countErr)
		}
		if count == 0 {
			return ErrPortfolioNotFound
		}
		return ErrProjectNotFound
	}

	return nil
}

// GetTechnologyStats menghasilkan 10 teknologi paling banyak digunakan melalui aggregation pipeline
// dengan dua tahap $unwind (projects, lalu projects.technologies).
func (r *portfolioMongoRepository) GetTechnologyStats(ctx context.Context, limit int) ([]model.TechnologyStat, error) {
	if limit <= 0 {
		limit = 10
	}

	pipeline := mongo.Pipeline{
		{{Key: "$unwind", Value: "$projects"}},
		{{Key: "$unwind", Value: "$projects.technologies"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$projects.technologies",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "count", Value: -1},
			{Key: "_id", Value: 1},
		}}},
		{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("agregasi statistik teknologi: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		Technology string `bson:"_id"`
		Count      int    `bson:"count"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("membaca statistik teknologi: %w", err)
	}

	stats := make([]model.TechnologyStat, 0, len(results))
	for _, res := range results {
		stats = append(stats, model.TechnologyStat{
			Technology: res.Technology,
			Count:      res.Count,
		})
	}
	return stats, nil
}
