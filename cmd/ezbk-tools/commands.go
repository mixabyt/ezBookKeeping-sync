package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"ezbkTools/internal/adapters/sqlite"
	"ezbkTools/internal/app"
	"ezbkTools/internal/config"
)

const usage = "використання: ezbk-tool adduser <логін>"

// runCommand виконує підкоманду і повертає код виходу процесу.
// Підкоманди живуть в основному бінарнику, бо в реліз іде лише він.
func runCommand(args []string) int {
	if len(args) == 2 && args[0] == "adduser" {
		return addUser(args[1])
	}
	fmt.Fprintln(os.Stderr, usage)
	return 2
}

// addUser створює користувача й один раз виводить його тимчасовий пароль.
// Запускати з того самого каталогу, що й сервіс: звідти читається .env.
func addUser(login string) int {
	cfg := config.NewConfig()

	db, err := sqlite.Open(cfg.DBDatabase)
	if err != nil {
		fmt.Fprintln(os.Stderr, "не вдалося відкрити БД:", err)
		return 1
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, password, err := app.CreateUser(ctx, sqlite.NewUser(db), login)
	if err != nil {
		fmt.Fprintln(os.Stderr, "не вдалося створити користувача:", err)
		return 1
	}

	fmt.Printf("Користувача %q створено (id %d).\n", user.Login, user.ID)
	fmt.Printf("Тимчасовий пароль: %s\n", password)
	fmt.Println("Більше він ніде не показується; під час першого входу його доведеться змінити.")
	return 0
}
