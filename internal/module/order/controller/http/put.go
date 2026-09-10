package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// orderOwnerCheck ties the declaration to the rule that decides it.
//
// The sentence is what the generated documentation prints, so it is written for
// a caller. The predicate is the module's own (service.IsOwner) — what "owner"
// means is a fact about orders, and an authorization framework has no business
// defining it. All this does is hand it the one thing it does not know about,
// which is that a verified caller is identified by a Subject.
var orderOwnerCheck = restkit.NewResourceCheck[dto.Order](
	"Only the caller the order belongs to can update it.",
	func(p *authkit.Principal, order *dto.Order) bool {
		return service.IsOwner(p.Subject, order)
	},
)

// NewPut は注文を更新するエンドポイント(型付き Endpoint)。
//
// このエンドポイントは **リソース単位の認可** の実例。`orders:write` を持っているかは
// トークンを見れば分かるが、「その注文が自分のものか」は注文を読まないと分からない。
// スコープの宣言だけでは閉じないので、宣言が判定関数への参照を持ち(`Resource`)、
// ハンドラがリソースを取得した直後に `restkit.Authorize` を呼ぶ。
//
// ⚠ **呼び忘れは黙って通らない**。`Resource` を宣言したエンドポイントが
// `Authorize` を呼ばずに成功を返すと、アダプタが 500 と構造化ログ
// (`type: authorization_drift_log`)で拒否する。宣言した制約が実装から
// 抜け落ちた状態は、生成ドキュメントが嘘をついている状態でもあるため。
func NewPut(svc service.OrderService) *restkit.Endpoint[dto.UpdateOrder, dto.Order] {
	return &restkit.Endpoint[dto.UpdateOrder, dto.Order]{
		Summary:     "Update an order",
		Description: "Replaces the mutable fields of an order the caller owns.",
		Task:        "update order",
		IDPrefix:    "ord",
		Success:     http.StatusOK,
		Security: &restkit.SecuritySpec{
			// スコープは「注文を書けるか」まで。「その注文が自分のものか」は
			// 下の Resource が決める。
			Scopes:   []string{"orders:write"},
			Resource: orderOwnerCheck,
		},
		PathRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		BodyRules: []*rule.RuleSet{
			// 小数を取り得るので MinValue(整数専用)ではなく MinFloatValue を使う。
			// ⚠ 閾値も float で渡すこと(`0` ではなく `0.0`)。整数リテラルを渡すと
			//    型が合わず、値によらずこのフィールドが不正と判定される。
			{Field: "amount", Rules: []rule.Rule{vkit.NotNil(), vkit.MinFloatValue(0.0)}},
			{Field: "status", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(32)}},
		},
		FieldDocs: map[string]string{
			"id":           "Order identifier",
			"ownerSubject": "Caller the order belongs to",
			"amount":       "Order total amount",
			"status":       "Order lifecycle status",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"replaces the order's amount and status"},
			// PUT は置き換えなので、同じ本文を二度送っても結果は同じ。
			Idempotent:    new(true),
			Preconditions: []string{"the order exists and belongs to the caller"},
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         "order_not_found",
				Condition:    "No order exists under that id",
				CallerAction: "Do not retry; check the id.",
			},
		},
		Related: []string{
			"Read the result: GET /order",
		},
		Handle: func(r *rest.Request, body *dto.UpdateOrder) (*rest.Result[dto.Order], error) {
			id, err := orderID(r)
			if err != nil {
				return nil, err
			}

			// 1. リソースを取得する。
			order, err := svc.FindOrder(id)
			if err != nil {
				if errors.Is(err, service.ErrOrderNotFound) {
					return nil, restkit.NewError(
						http.StatusNotFound, "order_not_found", "no order exists under that id")
				}
				// 型不明の error は 500 internal_error に丸められる(内部詳細を漏らさない)。
				return nil, err
			}

			// 2. 取得した直後に認可する —— 変更する前であり、応答する前でもある。
			//    拒否は 403 で、スコープ不足の 403 と同じ本文になる。
			if err := restkit.Authorize(r, order); err != nil {
				return nil, err
			}

			// 3. 認可された後にだけ変更する。
			updated, err := svc.UpdateOrder(id, body)
			if err != nil {
				return nil, err
			}
			return rest.NewResult(updated), nil
		},
	}
}

// orderID reads the path value as the id the service takes.
//
// The declared PathRules have already refused an empty or over-long value, so
// what is left is a value that is not a number at all — which is a bad request
// rather than a missing order.
func orderID(r *rest.Request) (uint, error) {
	raw, _ := r.PathValue("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, restkit.NewError(
			http.StatusBadRequest, "invalid_order_id", "the order id has to be a number")
	}
	return uint(id), nil
}
