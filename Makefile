start:
	go run ./main.go -c wooh.yaml -p images

autofill:
	go run ./main.go -a
build:
	go build -o ./dist/wooh ./main.go

install-local:
	go build -o $(GOBIN)/wooh ./main.go

install-win-local:
	go build -o $(GOBIN)/wooh.exe ./main.go

install:
	go install github.com/asolopovas/woo-helpers@latest

tag-push:
	$(eval VERSION=$(shell cat version))
	git tag $(VERSION)
	git push origin $(VERSION)
	if git rev-parse latest >/dev/null 2>&1; then git tag -d latest; fi
	git tag latest
	git push origin latest --force
