package proposals

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
)

// A role from the role master narrows what its base decides; it never widens it.
func TestAuthorizeRoleNarrowing(t *testing.T) {
	ctx := context.Background()
	admin := Decider{SalesUserID: uuid.New(), Role: "admin", RoleName: "Admin cabang", Decide: []string{domain.KindTransfer}}
	if err := authorize(ctx, nil, domain.KindTransfer, nil, nil, admin); err != nil {
		t.Fatalf("allowed kind refused: %v", err)
	}
	if err := authorize(ctx, nil, domain.KindPushStock, nil, nil, admin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("kind outside the role decided: %v", err)
	}
	wide := Decider{SalesUserID: uuid.New(), Role: "admin", Decide: []string{domain.KindCreditLimit}}
	if err := authorize(ctx, nil, domain.KindCreditLimit, nil, nil, wide); !errors.Is(err, ErrForbidden) {
		t.Fatalf("role widened its base: %v", err)
	}
	if err := authorize(ctx, nil, domain.KindPushStock, nil, nil, Decider{Role: "admin"}); err != nil {
		t.Fatalf("no role row = whole base: %v", err)
	}
}
