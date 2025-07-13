package utils

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
)

// 私钥字符串
var privateKey = `
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQChInyNJKzz6jhr
2Dk5pqMT2TqgWokAy68AC04+DFtIB8R+5t2Y/QmlPn3Oh4yR/wbyNeoWTErMori5
WLqhEr0nonC9WOkPei4fMREpo4Y87IRvpuUzZCZBENvaLEt5vm3QtGm1r4ALJe1+
9KzENvytkc0bEJBmowk0wW88LE6tKS4QHxQrTMcSV/w5n0R/GUBvUabucTpV6HUH
DNkw2GUH66SIwSrRv6xGHOSxIFIE7Idp982Z3DRTvZ4zYkJX8mSp3x+R9AYmJVu2
9K8jyglV2/MeKW6dNhUknLj1iFqQMw4Yug9oWIWUOg3SWCY8Z4nGR2DVk7sBL0HJ
mgO/YbarAgMBAAECggEAdWybVYQnoazLJxQwR6n+54UDaz3u3yyPwDl88Eyy7J/0
ewIk9MtZjxkmNy6iqvYtiq7tgwhf7habBT766kysmciP3fyAAu5n1AU+25g2SAmY
TYFTQAs4sWvmu2xSKEs03cXLz0Iwzm76Tu1hRbBInPhGfvWoNZOULuTT+gbT4u2a
IU+NfO085roIAomz+QPjtHKCdRz6pEAeLNHHgFgx+cn/NZmiVMnfiY9TKo0GJD+b
6Nv3jIFE2Fd/iyrj+4phu8FLwzkpitx/uDWlkqUdpjgGYpuTkR0ipmwOyENU4Gem
G+iU52lz1L9cNB8Vb7zZw7hAOf/IQbSIKxt/Dre9mQKBgQDV14MME813u6Csc+vu
R6QUyRs264pR9NrERlAQYESTIXnPCHxYe+BH0V3E350ruhKrzXoerBN1hiQ49u20
G4BY0nQrpQmw/zqb7dJk/qMkaxjyGv3e21lk/b0qkrCUkT9Rd2BZmDsETeRIjbAD
E/3bDnvncPCA/EvYl6y2JBT1/QKBgQDA5t9wEpgaGze6eyK8bWSq9Wt7qUKfc6Tj
1hvmeOrmmJ6QtFbPnDQM5kLhb8BI6wm2mgVCAZB/liKIWPmPABTxUte4XW7E/PIv
olcCQd5R/Ev+FnDBi36jc0/bjOzwto8kyZ+ldtgDcR/29eAuWztVydT7yl1Dabn1
975IPWkrxwKBgQDGHirtj5M3MQBFhgi59GnSUBgEo+i80au1WKdo5Kfj4InoBCag
G/TI1PKZKcuF7ZjKz04rCKXmpmb819mWmjwpDqJOpVL7RxvXx1i79SbU4Nx1wgge
5v5FkMgnn0w1+PO+2GjN2TokXL35cjv2PhldUGf/HyXTeuwSOUPsZDV/SQKBgCrS
FT93oTQKXrCSrP9O+U3J9PYaeKOUtEGvQbpDlUFjF6/fmHW1owhKBQauG+0T37Ad
OJWSa1UnKrtBpQRNbFi1nxVaCEDKNajFTLM/k+53JxdcO+N657241z1RZzd4DwaH
i1zbqM/6yLG1mvIvZliA2TqbjWBtk846FI9Mso/5AoGBAKzvskVHs2GQD3cmsEp9
e1Szi2oXt7q+yecysibMP8eWMzJAo+BOPpapzJ2kmNR+ZxzZm92p+wj5pPmL6ol8
LWPrUKtbvXEjVKheD/x8b2+ymy9CUz6/SS/li8Rd+vl29ewescW2ngAV2kWDjLHL
sjLQDIWFJ9FeoVNMOWr8M1QD
-----END PRIVATE KEY-----`

// CheckClientID 校验clientID的utils函数
func CheckClientID(clientID string) bool {
	if clientID == "" {
		return false
	}
	decodedClientID, err := base64.StdEncoding.DecodeString(clientID)
	if err != nil {
		return false
	}
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil {
		return false
	}
	privateKeyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return false
	}
	rsaPrivate, ok := privateKeyInterface.(*rsa.PrivateKey)
	if !ok {
		return false
	}
	plaintext, err := rsa.DecryptPKCS1v15(nil, rsaPrivate, decodedClientID)
	if err != nil {
		return false
	}
	return len(plaintext) == 32
}
