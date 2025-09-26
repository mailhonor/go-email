module github.com/mailhonor/go-email

go 1.24.7

// 替换本地依赖
// replace github.com/mailhonor/go-utils => ../go-utils

require github.com/mailhonor/go-utils v0.0.0-20250926032256-5528a6abcc3d

require (
	github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d // indirect
	golang.org/x/text v0.29.0 // indirect
)
