package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	match, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !match {
		t.Fatalf("expected password to match: match=%v err=%v", match, err)
	}
	match, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatal(err)
	}
	if match {
		t.Fatal("wrong password matched")
	}
}
