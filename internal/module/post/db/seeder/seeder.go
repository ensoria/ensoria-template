package seeder

import (
	"time"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
	"github.com/ensoria/ensoria-template/internal/module/post/model"
	"github.com/ensoria/gofake/pkg/faker"
)

type PostSeeder struct{}

func (s *PostSeeder) TableName() string { return "posts" }

func (s *PostSeeder) Seed(f faker.Faker) []model.Post {
	now := time.Now()
	return []model.Post{
		{UserID: 1, Title: f.Lorem.Sentence(5), Content: f.Lorem.Paragraphs(2, 3), CreatedAt: now, UpdatedAt: now},
		{UserID: 2, Title: f.Lorem.Sentence(5), Content: f.Lorem.Paragraphs(2, 3), CreatedAt: now, UpdatedAt: now},
		// NO TITLE: ゼロ値の場合は、カラムに設定された`DEFAULT`が適用される
		{UserID: 3, Content: f.Lorem.Paragraphs(2, 3), CreatedAt: now, UpdatedAt: now},
	}
}

func init() {
	encliseeder.Add("post", &PostSeeder{})
}
