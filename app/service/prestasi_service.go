package service

import (
	"context"

	"api-students/app/model"
	"api-students/app/repository"
)

type PrestasiService struct {
	repository repository.PrestasiRepository
}

func NewPrestasiService(
	prestasiRepository repository.PrestasiRepository,
) *PrestasiService {
	return &PrestasiService{
		repository: prestasiRepository,
	}
}

func (s *PrestasiService) FindByNIM(
	ctx context.Context,
	nim string,
) ([]model.Prestasi, error) {
	return s.repository.FindByNIM(ctx, nim)
}
