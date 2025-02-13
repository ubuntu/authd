//go:build generate

//go:generate go run -tags generate ../../internal/cmd/types-parser -source-package "github.com/ubuntu/authd/internal/proto/authd" -types "authd.UILayout" -types-aliases "UILayout" -package layouts -output ./uilayout.go -converter ../../internal/proto/authd/uilayout.go -converter-import "github.com/ubuntu/authd/brokers/layouts" -converter-package "authd"

package layouts
