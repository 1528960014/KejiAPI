package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("verify good password = false, want true")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("verify wrong password = true, want false")
	}
	if VerifyPassword("$argon2$v=19$m=1,t=1,p=1$xx$yy", "x") {
		t.Fatal("verify malformed hash = true, want false")
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	h1, err1 := HashPassword("same-password")
	h2, err2 := HashPassword("same-password")
	if err1 != nil || err2 != nil {
		t.Fatalf("hash: %v / %v", err1, err2)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical (missing salt?)")
	}
}

func TestAccessSignVerify(t *testing.T) {
	svc := NewTokenService("0123456789abcdef0123")
	token, err := svc.SignAccess(7, "alice@example.com")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := svc.VerifyAccess(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != 7 || claims.Email != "alice@example.com" {
		t.Fatalf("claims = %+v, want uid=7 email=alice@example.com", claims)
	}
}

func TestAccessTamperedOrForeign(t *testing.T) {
	svc := NewTokenService("0123456789abcdef0123")
	token, _ := svc.SignAccess(7, "alice@example.com")
	if _, err := svc.VerifyAccess(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	other := NewTokenService("ffffffffffffffffffff")
	if _, err := other.VerifyAccess(token); err == nil {
		t.Fatal("token signed with another secret accepted")
	}
}

func TestAccessExpired(t *testing.T) {
	svc := NewTokenService("0123456789abcdef0123")
	token, err := svc.signWithTTL(7, "alice@example.com", -time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := svc.VerifyAccess(token); err != ErrTokenExpired {
		t.Fatalf("expired token verify = %v, want ErrTokenExpired", err)
	}
}

func TestRefreshTokenUniqueness(t *testing.T) {
	a, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	b, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	if a == b {
		t.Fatal("two refresh tokens are identical")
	}
}
