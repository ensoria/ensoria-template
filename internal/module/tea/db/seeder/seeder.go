package seeder

import (
	"time"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
	"github.com/ensoria/ensoria-template/internal/module/tea/model"
	"github.com/ensoria/gofake/pkg/faker"
)

type TeaSeeder struct{}

func (s *TeaSeeder) TableName() string { return "teas" }

func (s *TeaSeeder) Seed(f faker.Faker) []model.Tea {
	now := time.Now()
	return []model.Tea{
		{Name: "Green Tea", Price: 100, CreatedAt: now, UpdatedAt: now},
		{Name: "Black Tea", Price: 150, CreatedAt: now, UpdatedAt: now},
		{Name: "Oolong Tea", Price: 200, CreatedAt: now, UpdatedAt: now},
	}
}

func init() {
	encliseeder.Add("tea", &TeaSeeder{})
}
