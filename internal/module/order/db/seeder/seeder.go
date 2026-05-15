package seeder

import (
	"time"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
	"github.com/ensoria/ensoria-template/internal/module/order/model"
	"github.com/ensoria/gofake/pkg/faker"
)

type OrderSeeder struct{}

func (s *OrderSeeder) TableName() string { return "orders" }

func (s *OrderSeeder) Seed(_ faker.Faker) []model.Order {
	now := time.Now()
	return []model.Order{
		{UserID: 1, OrderDetailID: 1, Total: 1000, CreatedAt: now, UpdatedAt: now},
		{UserID: 2, OrderDetailID: 2, Total: 2000, CreatedAt: now, UpdatedAt: now},
		{UserID: 3, OrderDetailID: 3, Total: 3000, CreatedAt: now, UpdatedAt: now},
	}
}

type OrderDetailSeeder struct{}

func (s *OrderDetailSeeder) TableName() string { return "order_details" }

func (s *OrderDetailSeeder) Seed(_ faker.Faker) []model.OrderDetail {
	now := time.Now()
	return []model.OrderDetail{
		{UserID: 1, OrderID: 1, ProductName: "Product A", Price: 1000, Quantity: 1, CreatedAt: now, UpdatedAt: now},
		{UserID: 1, OrderID: 1, ProductName: "Product B", Price: 500, Quantity: 2, CreatedAt: now, UpdatedAt: now},
		{UserID: 2, OrderID: 2, ProductName: "Product C", Price: 2000, Quantity: 1, CreatedAt: now, UpdatedAt: now},
		{UserID: 3, OrderID: 3, ProductName: "Product D", Price: 1500, Quantity: 2, CreatedAt: now, UpdatedAt: now},
	}
}

func init() {
	encliseeder.Add("order", &OrderSeeder{})
	encliseeder.Add("order", &OrderDetailSeeder{})
}
