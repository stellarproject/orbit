# Copyright (c) 2019 Stellar Project

# Permission is hereby granted, free of charge, to any person
# obtaining a copy of this software and associated documentation
# files (the "Software"), to deal in the Software without
# restriction, including without limitation the rights to use, copy,
# modify, merge, publish, distribute, sublicense, and/or sell copies
# of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED,
# INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
# IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
# HOLDERS BE LIABLE FOR ANY CLAIM,
# DAMAGES OR OTHER LIABILITY,
# WHETHER IN AN ACTION OF CONTRACT,
# TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH
# THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

PACKAGES=$(shell go list ./... | grep -v /vendor/)
REVISION=$(shell git rev-parse HEAD)
VERSION=2
GO_LDFLAGS=-s -w -X github.com/stellarproject/orbit/version.Version=$(VERSION) -X github.com/stellarproject/orbit/version.Revision=$(REVISION)
VAB_ARGS=""

all: FORCE
	go build -o build/orbit-server -v -ldflags '${GO_LDFLAGS}' github.com/stellarproject/orbit/cmd/orbit-server
	go build -o build/ob -v -ldflags '${GO_LDFLAGS}' github.com/stellarproject/orbit/cmd/ob
	go build -o build/orbit-log -v -ldflags '${GO_LDFLAGS}' github.com/stellarproject/orbit/cmd/orbit-log
	go build -o build/orbit-syslog -v -ldflags '${GO_LDFLAGS}' github.com/stellarproject/orbit/cmd/orbit-syslog
	gcc -static -o build/orbit-network cmd/orbit-network/main.c

clean:
	@rm -fr build/

FORCE:

install:
	@install build/ob /usr/local/bin/
	@install build/orbit-log /usr/local/bin/
	@install build/orbit-syslog /usr/local/bin/
	@install build/orbit-server /usr/local/bin/
	@install build/orbit-network /usr/local/bin/

protos:
	protobuild --quiet ${PACKAGES}

