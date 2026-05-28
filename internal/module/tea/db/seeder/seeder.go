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
	notAvailable := false
	bestBefore := time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC)
	return []model.Tea{
		{Name: "Green Tea", Price: 100, BestBefore: bestBefore, CreatedAt: now, UpdatedAt: now},
		// IsAvailable: &notAvailable で明示的に false を挿入。
		// *bool の nil はゼロ値としてスキップされ、DB の DEFAULT TRUE が使われる。
		{Name: "Black Tea", Price: 150, BestBefore: bestBefore, IsAvailable: &notAvailable, CreatedAt: now, UpdatedAt: now},
		{Name: "Oolong Tea", Price: 200, BestBefore: bestBefore},
		// priceが0のゼロ値でもINSERTに含めるため、seed:"force" タグをモデルのPriceフィールドに付けている。
		{Name: "Yomogi Tea", Price: 0, BestBefore: bestBefore},
	}
}

func init() {
	encliseeder.Add("tea", &TeaSeeder{})
}
