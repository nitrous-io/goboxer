FROM nitrousio/goboxer-ci

RUN curl -s https://storage.googleapis.com/golang/go1.3.3.linux-amd64.tar.gz | tar -C /usr/local -xz

ENV PATH /opt/go/bin:/usr/local/go/bin:$PATH
ENV GOBIN /opt/go/bin
ENV GOPATH /opt
RUN go get github.com/tools/godep

ENV GOPATH /opt:/opt/src/github.com/nitrous-io/goboxer/Godeps/_workspace
WORKDIR /opt/src/github.com/nitrous-io/goboxer

ADD . /opt/src/github.com/nitrous-io/goboxer
