package ui

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
)

// realKeys is the Keys page against the real seams: the Credential Manager
// holds the key and Windows Hello gates every write and delete. Biometrics
// only gate access to a key kept in the OS store; they encrypt nothing.
type realKeys struct {
	store    secrets.Store
	verifier hello.Verifier
}

func newRealKeys() *realKeys {
	return &realKeys{store: secrets.New(), verifier: hello.New()}
}

// newRealKeysWith injects the two seams so tests can drive consent outcomes
// without touching the real credential store or prompting a human.
func newRealKeysWith(store secrets.Store, verifier hello.Verifier) *realKeys {
	return &realKeys{store: store, verifier: verifier}
}

func (k *realKeys) accounts() ([]accountRow, error) {
	list, err := k.store.List()
	if err != nil {
		return nil, err
	}
	rows := make([]accountRow, 0, len(list))
	for _, acc := range list {
		rows = append(rows, accountRow{
			userID:  acc.UserID,
			email:   acc.Email,
			created: when(acc.CreatedAt),
		})
	}
	return rows, nil
}

// save stores one account's key. The base64 decode comes before the prompt so
// a malformed paste never makes the user answer a pointless Hello dialog, and
// the consent comes before Put so a refusal leaves the store untouched. The
// reference stores first and verifies after, which can leave a key behind when
// the user cancels.
func (k *realKeys) save(userID, keyB64 string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return fmt.Errorf("密钥不是合法的 base64：%v", err)
	}
	if err := k.verify("保存 biowarden 密钥"); err != nil {
		return err
	}
	acc := secrets.Account{UserID: strings.TrimSpace(userID), CreatedAt: time.Now()}
	return k.store.Put(acc, secrets.Key(key))
}

// remove deletes one account's key behind the same consent as save: the
// reference unlocks before it touches stored key material, so deletion is not
// a silent bypass of the gate.
func (k *realKeys) remove(userID string) error {
	if err := k.verify("删除 biowarden 密钥"); err != nil {
		return err
	}
	return k.store.Remove(userID)
}

func (k *realKeys) helloAvailable() (bool, error) {
	return k.verifier.Available()
}

// verify turns a Hello outcome into one line a human can act on. A refused or
// unusable check must read differently from a broken call.
func (k *realKeys) verify(reason string) error {
	result, err := k.verifier.Verify(reason)
	if err != nil {
		return fmt.Errorf("Windows Hello 调用失败：%v", err)
	}
	switch result {
	case hello.Verified:
		return nil
	case hello.Canceled:
		return errors.New("已取消 Windows Hello 验证")
	case hello.Unavailable:
		return errors.New("此电脑未配置 Windows Hello")
	case hello.Locked:
		return errors.New("Windows Hello 已被锁定，请稍后再试")
	case hello.Busy:
		return errors.New("Windows Hello 暂时不可用（设备忙）")
	default:
		return fmt.Errorf("Windows Hello 返回未知结果：%v", result)
	}
}
