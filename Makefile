appname := mxproxy
# version := 1.0.10

# These are the values we want to pass for VERSION and BUILD
# git tag 1.0.1
# git commit -am "One more change after the tags"

sources  := $(wildcard *.go)
DATE     := $(shell date -u +%Y-%m-%d)
GIT      := $(shell git rev-parse --short HEAD) 
VERSION  := $(shell git describe --tags 2> /dev/null || echo 0.1)
REVISION := $(shell git log --oneline | wc -l | tr -d ' ')

FLAGS    := -ldflags "-X main.date=$(DATE) -X main.version=$(VERSION).$(REVISION) -X main.git=$(GIT)"

build = GOOS=$(1) GOARCH=$(2) go build -o build/$(appname)$(3) $(FLAGS)
tar = cd ./build && tar -czf $(1)_$(2).tar.gz $(appname)$(3) && rm $(appname)$(3)
zip = cd ./build && zip $(1)_$(2).zip $(appname)$(3) && rm $(appname)$(3)

.PHONY: all windows darwin linux clean build min

info:
	@echo "---------------------"
	@echo "Date:      $(DATE)"
	@echo "Version:   $(VERSION)"
	@echo "Revision:  $(REVISION)"
	@echo "GIT:       $(GIT)"
	@echo "---------------------"

clean:
	rm -rf build/

build: info
	@go build -race $(FLAGS)

all: linux darwin windows

min: linux darwin

##### LINUX BUILDS #####
linux: info clean build/linux_arm.tar.gz build/linux_arm64.tar.gz build/linux_386.tar.gz build/linux_amd64.tar.gz

build/linux_386.tar.gz: $(sources)
	$(call build,linux,386,)
	$(call tar,linux,386)

build/linux_amd64.tar.gz: $(sources)
	$(call build,linux,amd64,)
	$(call tar,linux,amd64)

build/linux_arm.tar.gz: $(sources)
	$(call build,linux,arm,)
	$(call tar,linux,arm)

build/linux_arm64.tar.gz: $(sources)
	$(call build,linux,arm64,)
	$(call tar,linux,arm64)

##### DARWIN (MAC) BUILDS #####
darwin: info clean build/darwin_amd64.tar.gz

build/darwin_amd64.tar.gz: $(sources)
	$(call build,darwin,amd64,)
	$(call tar,darwin,amd64)


##### WINDOWS BUILDS #####
windows: info clean build/windows_386.zip build/windows_amd64.zip

build/windows_386.zip: $(sources)
	$(call build,windows,386,.exe)
	$(call zip,windows,386,.exe)

build/windows_amd64.zip: $(sources)
	$(call build,windows,amd64,.exe)
	$(call zip,windows,amd64,.exe)