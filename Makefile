PKGDIR = pkg
ZIPPED_PKGDIR = bz2pkg
COMMIT = $$(git describe --always)

all: build

build:
	go build -ldflags "-X main.GitCommit=\"$(COMMIT)\"" -o gohakai

build_all:
	@if [ ! -d $(PKGDIR) ]; then \
		mkdir $(PKGDIR); \
	fi
	@if [ ! -d $(ZIPPED_PKGDIR) ]; then \
		mkdir $(ZIPPED_PKGDIR); \
	fi
	GOOS=darwin GOARCH=386 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.darwin.386 main.go indicator.go statistics.go config.go remote.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.darwin.amd64 main.go indicator.go statistics.go config.go remote.go
	GOOS=linux GOARCH=386 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.linux.386 main.go indicator.go statistics.go config.go remote.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.linux.amd64 main.go indicator.go statistics.go config.go remote.go
	GOOS=windows GOARCH=386 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.windows.386 main.go indicator.go statistics.go config.go remote.go
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.GitCommit \"$(COMMIT)\"" -o $(PKGDIR)/gohakai.windows.amd64 main.go indicator.go statistics.go config.go remote.go
	bzip2 -c  $(PKGDIR)/gohakai.darwin.386   > $(ZIPPED_PKGDIR)/gohakai.darwin.386.bz2
	bzip2 -c  $(PKGDIR)/gohakai.darwin.amd64 > $(ZIPPED_PKGDIR)/gohakai.darwin.amd64.bz2
	bzip2 -c  $(PKGDIR)/gohakai.linux.386    > $(ZIPPED_PKGDIR)/gohakai.linux.386.bz2
	bzip2 -c  $(PKGDIR)/gohakai.linux.amd64  > $(ZIPPED_PKGDIR)/gohakai.linux.amd64.bz2
	bzip2 -c  $(PKGDIR)/gohakai.windows.386    > $(ZIPPED_PKGDIR)/gohakai.windows.386.bz2
	bzip2 -c  $(PKGDIR)/gohakai.windows.amd64  > $(ZIPPED_PKGDIR)/gohakai.windows.amd64.bz2

clean:
	rm -f gohakai
	rm -rf $(PKGDIR)

update-module:
	go get -u -v gopkg.in/yaml.v2
	go get -u -v golang.org/x/crypto/ssh
	go get -u -v golang.org/x/net/http2
