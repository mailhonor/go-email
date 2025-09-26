module github.com/mailhonor/go-email

go 1.24.0

toolchain go1.24.7

require github.com/mailhonor/go-utils v0.0.0-00010101000000-000000000000

require (
	github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d // indirect
	golang.org/x/text v0.29.0 // indirect
)

// 替换本地依赖
replace github.com/mailhonor/go-utils => ../go-utils
