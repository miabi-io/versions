export GOWORK := off

.PHONY: test validate index

## test: run the unit tests
test:
	go test ./...

## validate: check every manifest and that v1/index.json is current
validate:
	go run ./cmd/versions validate -dir public

## index: regenerate v1/index.json from the component manifests
index:
	go run ./cmd/versions index -dir public
