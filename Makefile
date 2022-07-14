TARGETS = stacker

stacker: *.go *.json
	GOARCH=amd64 GOOS=linux go build
	zip stacker.zip stacker *.json

clean:
	rm -f ${TARGETS} ${TARGETS}.zip