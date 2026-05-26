package scim

import "testing"

func TestPrimaryEmail(t *testing.T) {
	cases := []struct {
		name string
		u    User
		want string
	}{
		{
			name: "primary flagged",
			u: User{
				UserName: "ignored",
				Emails: []Email{
					{Value: "alt@example.com"},
					{Value: "Primary@Example.com", Primary: true},
				},
			},
			want: "primary@example.com",
		},
		{
			name: "first email when none primary",
			u: User{
				Emails: []Email{
					{Value: "first@example.com"},
					{Value: "second@example.com"},
				},
			},
			want: "first@example.com",
		},
		{
			name: "username fallback",
			u:    User{UserName: "User@example.com"},
			want: "user@example.com",
		},
		{
			name: "no email",
			u:    User{UserName: "no-email"},
			want: "",
		},
	}
	for _, c := range cases {
		got := primaryEmail(c.u)
		if got != c.want {
			t.Errorf("%s: primaryEmail = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNewError(t *testing.T) {
	e := NewError(404, "not found")
	if e.Status != "404" {
		t.Errorf("status = %q, want 404", e.Status)
	}
	if e.Detail != "not found" {
		t.Errorf("detail = %q", e.Detail)
	}
	if len(e.Schemas) != 1 || e.Schemas[0] != SchemaError {
		t.Errorf("schemas = %v", e.Schemas)
	}
}
