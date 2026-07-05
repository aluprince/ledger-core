# Set it permanently in your shell session
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/ledgercore_test?sslmode=disable"

# Then run on the SAME line as the export (semicolon, not newline)
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/ledgercore_test?sslmode=disable" && go test ./... -v