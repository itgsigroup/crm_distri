package proposals

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
)

// The role master decides who decides what; releasing credit above the limit stays with policy holders (CEO).
func TestAuthorizeRoleNarrowing(t *testing.T) {
	ctx := context.Background()
	admin := Decider{SalesUserID: uuid.New(), Role: "admin", RoleName: "Admin cabang", Decide: []string{domain.KindTransfer}}
	if err := authorize(ctx, nil, domain.KindTransfer, nil, nil, admin); err != nil {
		t.Fatalf("allowed kind refused: %v", err)
	}
	if err := authorize(ctx, nil, domain.KindPushStock, nil, nil, admin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("kind outside the role decided: %v", err)
	}
	custom := Decider{SalesUserID: uuid.New(), Role: "admin", Decide: []string{domain.KindCreditLimit, domain.KindCreditRelease}}
	if err := authorize(ctx, nil, domain.KindCreditLimit, nil, nil, custom); err != nil {
		t.Fatalf("a ticked kind refused: %v", err)
	}
	if err := authorize(ctx, nil, domain.KindCreditRelease, nil, nil, custom); !errors.Is(err, ErrForbidden) {
		t.Fatalf("credit release above the limit without policy rights: %v", err)
	}
	if err := authorize(ctx, nil, domain.KindPushStock, nil, nil, Decider{Role: "admin"}); err != nil {
		t.Fatalf("no role row = whole base: %v", err)
	}
}
