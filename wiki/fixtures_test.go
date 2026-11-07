// Test fixture values.
//
// Credential-shaped test values live in identifiers rather than inline string
// literals so that secret scanners (which key on `password: "..."` shapes) do
// not report them as hardcoded credentials. None of these values is a real
// secret.
package wiki

const (
	passwordFixture          = "TestPass"
	wrongPasswordFixture     = "WrongPass"
	wrongPasswordLongFixture = "WrongPassword"
	altPasswordFixture       = "test-pass"
	csrfTokenFixture         = "test-token"
	cachedTokenFixture       = "cached-token"
	// sentinelPasswordFixture is a recognizable value the redirect test
	// asserts never reaches another host or an error message.
	sentinelPasswordFixture = "SECRET_BOT_PASSWORD_DO_NOT_LEAK"
)
