package repository

import (
	"context"

	model "api-students/app/model"
)

type StudentRepository interface {
	FindAll(ctx context.Context) ([]model.Student, error)
	FindByID(ctx context.Context, id int) (*model.Student, error)
	Create(ctx context.Context, student *model.Student) error
	Update(ctx context.Context, student *model.Student) error
	Delete(ctx context.Context, id int) error
}
