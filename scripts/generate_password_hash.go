// +build ignore

package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run generate_password_hash.go <password>")
		fmt.Println("Example: go run generate_password_hash.go @abcd4321")
		os.Exit(1)
	}

	password := os.Args[1]
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("Error generating hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Password: %s\n", password)
	fmt.Printf("Hash: %s\n", string(hash))
	fmt.Println("\nUse this hash to update admin_users table in Supabase:")
	fmt.Printf("UPDATE admin_users SET password_hash = '%s' WHERE username = 'datvt8x';\n", string(hash))
}
