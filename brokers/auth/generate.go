//go:build generate

//go:generate go run -tags generate ../../internal/cmd/types-parser -source-package "github.com/ubuntu/authd/internal/proto/authd" -types "authd.GAMResponse_AuthenticationMode" -types-aliases "Mode" -package auth -output ./authmode.go -converter ../../internal/proto/authd/authmode.go -converter-import "github.com/ubuntu/authd/brokers/auth" -converter-package "authd"

package auth
