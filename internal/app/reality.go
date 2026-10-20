package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/Spittingjiu/sui-go/internal/model"
	"github.com/google/uuid"
)

// Reality 的密钥就是一对 X25519 密钥。之前完全依赖 `xray x25519` 子进程，
// 在「一键安装只装了面板二进制、Xray 还没装好」的机器上会直接
// exit status 127（command not found），一键创建 Reality 就必然失败。
// 这里用 Go 标准库直接生成，xray 只作为可选来源，缺失也能正常创建。
func generateRealityKeypair() (privateKey, publicKey, shortID string) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", ""
	}
	privateKey = base64.RawURLEncoding.EncodeToString(priv.Bytes())
	publicKey = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	shortID = strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	return privateKey, publicKey, shortID
}

// realityKeypairFromXray 仅在 xray 二进制存在时使用，保持与已有节点一致。
func (a *App) realityKeypairFromXray() (privateKey, publicKey string, ok bool) {
	out, err := a.runBestEffort("xray x25519 2>/dev/null")
	if err != nil {
		return "", "", false
	}
	for _, ln := range strings.Split(out, "\n") {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "PrivateKey:") {
			privateKey = strings.TrimSpace(strings.TrimPrefix(s, "PrivateKey:"))
		}
		// xray 各版本输出过 "Password (PublicKey):" 和 "Public key:" 两种写法。
		if strings.HasPrefix(s, "Password (PublicKey):") {
			publicKey = strings.TrimSpace(strings.TrimPrefix(s, "Password (PublicKey):"))
		}
		if publicKey == "" && strings.HasPrefix(s, "Public key:") {
			publicKey = strings.TrimSpace(strings.TrimPrefix(s, "Public key:"))
		}
	}
	if privateKey == "" || publicKey == "" {
		return "", "", false
	}
	return privateKey, publicKey, true
}

// fillRealityKeys 补齐 Reality 必需字段：密钥对与 shortId。
// 先尝试 xray x25519（与历史节点保持一致），否则回退到内置生成器。
func (a *App) fillRealityKeys(req *model.AddInboundRequest) {
	if strings.TrimSpace(req.PrivateKey) != "" && strings.TrimSpace(req.PublicKey) != "" {
		if strings.TrimSpace(req.ShortID) == "" {
			req.ShortID = strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		}
		return
	}
	if pri, pub, ok := a.realityKeypairFromXray(); ok {
		if strings.TrimSpace(req.PrivateKey) == "" {
			req.PrivateKey = pri
		}
		if strings.TrimSpace(req.PublicKey) == "" {
			req.PublicKey = pub
		}
	} else {
		pri, pub, _ := generateRealityKeypair()
		if strings.TrimSpace(req.PrivateKey) == "" {
			req.PrivateKey = pri
		}
		if strings.TrimSpace(req.PublicKey) == "" {
			req.PublicKey = pub
		}
	}
	if strings.TrimSpace(req.ShortID) == "" {
		req.ShortID = strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	}
}
