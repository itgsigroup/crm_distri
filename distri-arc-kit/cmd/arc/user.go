package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/google/uuid"

	"distri-arc/internal/auth"
	"distri-arc/internal/config"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// arc ctl user add|passwd|totp-reset
func runUserCtl(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl user add --email --name --role --password [--sales] | passwd --email --password")
	}
	fs := flag.NewFlagSet("user "+args[0], flag.ExitOnError)
	email := fs.String("email", "", "login email")
	name := fs.String("name", "", "display name")
	role := fs.String("role", "sales", "ceo | admin | finance | sales | warehouse")
	password := fs.String("password", "", "password (≥ 10 characters)")
	sales := fs.String("sales", "", "sales_users name this account belongs to")
	branch := fs.String("branch", "", "branch of a new person profile (default: Semua cabang)")
	_ = fs.Parse(args[1:])
	if args[0] == "totp-reset" {
		n, err := st.Q.ResetTOTPByEmail(ctx, *email)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("pengguna tidak ditemukan")
		}
		fmt.Println("2FA dimatikan untuk", *email, "— aktifkan lagi di Pengaturan → Keamanan akun")
		return nil
	}
	if *email == "" || len(*password) < 10 {
		return errors.New("--email and --password (≥ 10 karakter) wajib")
	}
	hash, err := auth.Hash(*password)
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		if !slices.Contains(auth.Roles, *role) || *name == "" {
			return errors.New("--name dan --role (ceo|admin|finance|sales|warehouse) wajib")
		}
		var sid *uuid.UUID
		if *sales != "" {
			id, err := st.Q.SalesUserByName(ctx, *sales)
			if err != nil {
				return fmt.Errorf("sales %q tidak ditemukan", *sales)
			}
			sid = &id
		} else { // a person profile records this account's decisions
			b := *branch
			if b == "" {
				b = "Semua cabang"
			}
			id, err := st.Q.CreatePersonProfile(ctx, gen.CreatePersonProfileParams{Name: *name, Branch: b, Role: *role, Email: email})
			if err != nil {
				return err
			}
			sid = &id
		}
		id, err := st.Q.CreateUser(ctx, gen.CreateUserParams{Email: email, Name: name, Role: role, PasswordHash: &hash, SalesUserID: sid})
		if err != nil {
			return err
		}
		fmt.Printf("pengguna %s (%s) dibuat · %s\n", *email, *role, id)
	case "passwd":
		if err := st.Q.SetUserPassword(ctx, gen.SetUserPasswordParams{Lower: *email, PasswordHash: &hash}); err != nil {
			return err
		}
		fmt.Printf("kata sandi %s diperbarui\n", *email)
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
	return nil
}

// demoPasswords gives the sample accounts a password when APP_ENV=dev and ARC_DEMO_PASSWORD is set (login testing);
// accounts that already have one keep it. Never runs outside dev.
func demoPasswords(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger) error {
	pw := os.Getenv("ARC_DEMO_PASSWORD")
	if !cfg.IsDev() || pw == "" {
		return nil
	}
	emails, err := st.Q.UsersWithoutPassword(ctx)
	if err != nil {
		return err
	}
	hash, err := auth.Hash(pw)
	if err != nil {
		return err
	}
	for _, e := range emails {
		if err := st.Q.SetUserPassword(ctx, gen.SetUserPasswordParams{Lower: deref(e), PasswordHash: &hash}); err != nil {
			return err
		}
	}
	log.Info("demo passwords set", "accounts", len(emails))
	return nil
}
