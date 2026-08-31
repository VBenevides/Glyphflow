package platform

import "testing"

func TestParseEmailList(t *testing.T) {
	emails, err := ParseEmailList("Admin@Example.com, second@example.com;admin@example.com third@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 3 || emails[0] != "admin@example.com" || emails[1] != "second@example.com" || emails[2] != "third@example.com" {
		t.Fatalf("emails = %#v", emails)
	}
}

func TestNormalizeEmailRejectsInvalidAddress(t *testing.T) {
	if email, err := NormalizeEmail("admin@domain.com"); err != nil || email != "admin@domain.com" {
		t.Fatalf("development email rejected: %q, %v", email, err)
	}
	for _, value := range []string{"", "not-an-email", "Admin <admin@example.com>"} {
		if _, err := NormalizeEmail(value); err == nil {
			t.Fatalf("accepted invalid email %q", value)
		}
	}
}
