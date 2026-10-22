package service

import (
	"errors"
	"strings"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// NormalizeTechnologies merapikan nama teknologi: huruf kecil, trim spasi,
// menghapus duplikasi dengan urutan kemunculan pertama dipertahankan.
func NormalizeTechnologies(techs []string) []string {
	result := make([]string, 0, len(techs))
	seen := make(map[string]struct{}, len(techs))
	for _, t := range techs {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		result = append(result, t)
	}
	return result
}

type PortfolioService struct {
	portfolios repository.PortfolioRepository
	students   repository.StudentRepository
	perms      *helper.PermissionSet
}

func NewPortfolioService(
	portfolios repository.PortfolioRepository,
	students repository.StudentRepository,
	perms *helper.PermissionSet,
) *PortfolioService {
	return &PortfolioService{
		portfolios: portfolios,
		students:   students,
		perms:      perms,
	}
}

// GET /students/:id/portfolio
func (s *PortfolioService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	studentID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id mahasiswa harus berupa angka positif")
	}

	// Verifikasi apakah mahasiswa ada di PostgreSQL
	_, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
			return helper.NotFound("mahasiswa tidak ditemukan")
		}
		return helper.Internal(err)
	}

	portfolio, err := s.portfolios.FindByStudentID(ctx, studentID)
	if err != nil {
		if errors.Is(err, repository.ErrPortfolioNotFound) {
			return helper.NotFound("portfolio tidak ditemukan")
		}
		return helper.Internal(err)
	}

	return helper.Success(c, fiber.StatusOK, "portfolio ditemukan", portfolio)
}

// PUT /students/:id/portfolio
func (s *PortfolioService) Upsert(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	studentID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id mahasiswa harus berupa angka positif")
	}

	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
			return helper.NotFound("mahasiswa tidak ditemukan")
		}
		return helper.Internal(err)
	}

	// Pemeriksaan otorisasi: hanya pemilik data mahasiswa di PostgreSQL atau admin
	isOwner := (student.OwnerID == current.UserID)
	isAdmin := s.perms.Can(current.Role, "student:update:any") || current.Role == "admin"
	if !isOwner && !isAdmin {
		return helper.Forbidden("hanya pemilik data mahasiswa atau admin yang boleh mengelola portfolio")
	}

	var req model.UpsertPortfolioRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.Headline = strings.TrimSpace(req.Headline)
	req.Bio = strings.TrimSpace(req.Bio)

	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	// Petakan domain model
	projects := make([]model.Project, 0, len(req.Projects))
	for _, p := range req.Projects {
		projects = append(projects, model.Project{
			ID:           p.ID,
			Title:        strings.TrimSpace(p.Title),
			Technologies: NormalizeTechnologies(p.Technologies),
			Link:         strings.TrimSpace(p.Link),
			Year:         p.Year,
		})
	}

	certificates := make([]model.Certificate, 0, len(req.Certificates))
	for _, cr := range req.Certificates {
		certificates = append(certificates, model.Certificate{
			ID:     cr.ID,
			Name:   strings.TrimSpace(cr.Name),
			Issuer: strings.TrimSpace(cr.Issuer),
			Date:   strings.TrimSpace(cr.Date),
		})
	}

	saved, err := s.portfolios.Upsert(ctx, model.Portfolio{
		StudentID:    studentID,
		Headline:     req.Headline,
		Bio:          req.Bio,
		Projects:     projects,
		Certificates: certificates,
	})
	if err != nil {
		return translatePortfolioError(err)
	}

	return helper.Success(c, fiber.StatusOK, "portfolio berhasil disimpan", saved)
}

// POST /students/:id/portfolio/projects
func (s *PortfolioService) AddProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	studentID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id mahasiswa harus berupa angka positif")
	}

	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
			return helper.NotFound("mahasiswa tidak ditemukan")
		}
		return helper.Internal(err)
	}

	isOwner := (student.OwnerID == current.UserID)
	isAdmin := s.perms.Can(current.Role, "student:update:any") || current.Role == "admin"
	if !isOwner && !isAdmin {
		return helper.Forbidden("hanya pemilik data mahasiswa atau admin yang boleh mengelola portfolio")
	}

	var req model.CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Technologies = NormalizeTechnologies(req.Technologies)
	req.Link = strings.TrimSpace(req.Link)

	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	updated, err := s.portfolios.AddProject(ctx, studentID, model.Project{
		Title:        req.Title,
		Technologies: req.Technologies,
		Link:         req.Link,
		Year:         req.Year,
	})
	if err != nil {
		return translatePortfolioError(err)
	}

	return helper.Success(c, fiber.StatusCreated, "proyek berhasil ditambahkan", updated)
}

// DELETE /students/:id/portfolio/projects/:projectId
func (s *PortfolioService) DeleteProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	studentID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id mahasiswa harus berupa angka positif")
	}

	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
			return helper.NotFound("mahasiswa tidak ditemukan")
		}
		return helper.Internal(err)
	}

	isOwner := (student.OwnerID == current.UserID)
	isAdmin := s.perms.Can(current.Role, "student:update:any") || current.Role == "admin"
	if !isOwner && !isAdmin {
		return helper.Forbidden("hanya pemilik data mahasiswa atau admin yang boleh mengelola portfolio")
	}

	projectID := c.Params("projectId")
	if err := s.portfolios.DeleteProject(ctx, studentID, projectID); err != nil {
		return translatePortfolioError(err)
	}

	return helper.NoContent(c)
}

// GET /portfolios/stats/technologies
func (s *PortfolioService) GetTechnologyStats(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	stats, err := s.portfolios.GetTechnologyStats(ctx, 10)
	if err != nil {
		return helper.Internal(err)
	}

	return helper.Success(c, fiber.StatusOK, "statistik teknologi portfolio berhasil diambil", stats)
}

func translatePortfolioError(err error) error {
	switch {
	case errors.Is(err, repository.ErrInvalidID):
		return helper.BadRequest("id proyek tidak valid")
	case errors.Is(err, repository.ErrPortfolioNotFound):
		return helper.NotFound("portfolio tidak ditemukan")
	case errors.Is(err, repository.ErrProjectNotFound):
		return helper.NotFound("proyek tidak ditemukan")
	case errors.Is(err, repository.ErrProjectLimit):
		return helper.NewError(fiber.StatusConflict, "PROJECT_LIMIT", "jumlah proyek sudah mencapai batas maksimal 20")
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("data tidak ditemukan")
	case errors.Is(err, repository.ErrDuplicate):
		return helper.NewError(fiber.StatusConflict, "DUPLICATE_PORTFOLIO", "mahasiswa sudah memiliki portfolio")
	default:
		return helper.Internal(err)
	}
}
