package seeder

import (
	"github.com/ensoria/ensoria-template/internal/module/order/model"
	"github.com/ensoria/gofake/pkg/faker"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
)

type OrderSeeder struct{}

func (s *OrderSeeder) TableName() string { return "orders" }

func (s *OrderSeeder) Seed(f faker.Faker) []model.Order {
	return []model.Order{
		{UserID: 1, OrderDetailID: 1, Total: 1000},
		{UserID: 2, OrderDetailID: 2, Total: 2000},
		{UserID: 3, OrderDetailID: 3, Total: 3000},
	}
}

type OrderDetailSeeder struct{}

func (s *OrderDetailSeeder) TableName() string { return "order_details" }

func (s *OrderDetailSeeder) Seed(f faker.Faker) []model.OrderDetail {
	return []model.OrderDetail{
		{UserID: 1, OrderID: 1, ProductName: "Product A", Price: 1000, Quantity: 1},
		{UserID: 1, OrderID: 1, ProductName: "Product B", Price: 500, Quantity: 2},
		{UserID: 2, OrderID: 2, ProductName: "Product C", Price: 2000, Quantity: 1},
		{UserID: 3, OrderID: 3, ProductName: "Product D", Price: 1500, Quantity: 2},
	}
}

func init() {
	encliseeder.Add("order", &OrderSeeder{})
	encliseeder.Add("order", &OrderDetailSeeder{})
}
