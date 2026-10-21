// Команда hashpw рахує bcrypt-хеш пароля для AUTH_PASSWORD_HASH.
// Пароль читається з аргументу або зі stdin, щоб не лишався в історії шелла:
//
//	go run ./cmd/hashpw            # спитає пароль
//	echo -n 'pass' | go run ./cmd/hashpw
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := ""
	if len(os.Args) > 1 {
		password = os.Args[1]
	} else {
		fmt.Fprint(os.Stderr, "пароль: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr, "не вдалося прочитати пароль:", err)
			os.Exit(1)
		}
		password = strings.TrimRight(line, "\r\n")
	}

	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "пароль закороткий: мінімум 8 символів")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bcrypt:", err)
		os.Exit(1)
	}
	// лапки обовʼязкові: godotenv розгортає $... у незакавичених значеннях,
	// а bcrypt-хеш починається з "$2a$10$" і без лапок приїде покаліченим
	fmt.Printf("AUTH_PASSWORD_HASH='%s'\n", hash)
}
