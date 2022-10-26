package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"hms/gateway/pkg/common"
	"hms/gateway/pkg/config"
	"hms/gateway/pkg/infrastructure"
	"hms/gateway/pkg/user/roles"
	"log"
	"strings"

	"golang.org/x/crypto/scrypt"
)

func main() {
	var (
		cfgPath = flag.String("config", "./config.json", "config file path")
	)

	flag.Parse()

	cfg, err := config.New(*cfgPath)
	if err != nil {
		panic(err)
	}

	infra := infrastructure.New(cfg)

	_, userPrivKey, err := infra.Keystore.Get(cfg.DefaultUserID)
	if err != nil {
		log.Fatalf("Keystore.Get error: %v userID %s", err, cfg.DefaultUserID)
	}

	pwdHash, err := generateHashFromPassword(cfg.CreatingSystemID, cfg.DefaultUserID, "")
	if err != nil {
		log.Fatalf("generateHashFromPassword error: %v", err)
	}

	txHash, err := infra.Index.UserAdd(context.Background(), cfg.DefaultUserID, cfg.CreatingSystemID, uint8(roles.Patient), pwdHash, userPrivKey)
	if err != nil {
		log.Fatalf("Index.SetGroupAccess error: %v", err)
	}

	_, err = infra.Index.TxWait(context.Background(), txHash)
	if err != nil {
		log.Fatalf("index.TxWait error: %v txHash %s", err, txHash)
	}

	log.Println("txHash:", txHash)
}

func generateHashFromPassword(ehrSystemID, userID, password string) ([]byte, error) {
	salt := make([]byte, common.ScryptSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("rand.Read error: %w", err)
	}

	password = strings.Join([]string{userID, ehrSystemID, password}, "")

	pwdHash, err := scrypt.Key([]byte(password), salt, common.ScryptN, common.ScryptR, common.ScryptP, common.ScryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("generateHash scrypt.Key error: %w", err)
	}

	return append(pwdHash, salt...), nil
}
