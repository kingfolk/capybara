module github.com/kingfolk/capybara

go 1.15

require (
	github.com/fatih/color v1.12.0 // indirect
	github.com/llvm/llvm-project/bindings/go v0.0.0-20201007101048-176249bd6732
	github.com/rhysd/gocaml v0.0.0-20200704044627-535c093eec55
	github.com/rhysd/locerr v0.0.0-20170710120751-9e34f7a52ee7
	github.com/stretchr/testify v1.6.1
	google.golang.org/genproto v0.0.0-20211005153810-c76a74d43a8e
	honnef.co/go/tools v0.2.1
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
)

replace github.com/llvm/llvm-project/bindings/go => ../../../llvm.org/llvm/bindings/go
