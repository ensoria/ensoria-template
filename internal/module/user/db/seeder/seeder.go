package seeder

import (
	"time"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
	"github.com/ensoria/gofake/pkg/faker"

	"github.com/ensoria/ensoria-template/internal/module/user/model"
)

type UserSeeder struct{}

func (s *UserSeeder) TableName() string { return "users" }

func (s *UserSeeder) Seed(f faker.Faker) []model.User {
	now := time.Now()
	return []model.User{
		{Name: f.Person.Name(), CreatedAt: now, UpdatedAt: now},
		{Name: f.Person.Name(), CreatedAt: now, UpdatedAt: now},
		{Name: f.Person.Name(), CreatedAt: now, UpdatedAt: now},
	}
}

func init() {
	encliseeder.Add("user", &UserSeeder{})
}
