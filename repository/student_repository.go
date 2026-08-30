package repository

import (
	"context"

	"api-students/models"
)

type StudentRepository interface {
	FindAll(ctx context.Context) ([]models.Student, error)
	FindByID(ctx context.Context, id int) (*models.Student, error)
	Create(ctx context.Context, student *models.Student) error
	Update(ctx context.Context, student *models.Student) error
	Delete(ctx context.Context, id int) error
}