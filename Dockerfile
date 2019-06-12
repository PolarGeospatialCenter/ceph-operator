FROM golang:stretch

WORKDIR /go/src/github.com/PolarGeospatialCenter/ceph-operator
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

COPY Gopkg.lock Gopkg.toml ./
RUN dep ensure -vendor-only

COPY ./ .
RUN go build -o /bin/ceph-operator ./cmd/manager

FROM scratch
COPY --from=0 /bin/ceph-operator /bin/ceph-operator
ENTRYPOINT /bin/ceph-operator
