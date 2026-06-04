package config

import (
	"fmt"
	"os"
	"strings"
)

func NormalizePrivateKey(privateKey string) string {
	pk := strings.TrimSpace(privateKey)
	if strings.HasPrefix(pk, "0x") || strings.HasPrefix(pk, "0X") {
		return pk[2:]
	}
	return pk
}

func GetPrivateKey(file string) string {
	b, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	return NormalizePrivateKey(string(b))
}

func DeleteBlockchainPrivateKeyFileAfterRead() error {
	if appConfig == nil {
		return nil
	}

	file := appConfig.Blockchain.Account.PrivateKeyFile
	if err := os.Remove(file); err != nil {
		return fmt.Errorf("delete blockchain private key file %s: %w", file, err)
	}
	return nil
}
