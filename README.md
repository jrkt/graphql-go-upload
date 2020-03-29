# graphql-go-upload
A middleware for GraphQL in Go to support file uploads with a custom `Upload` scalar type

## Installation
```bash
go get github.com/jrkt/graphql-go-upload
```

## Usage
This middleware is designed to work with any GraphQL implementation. Simply wrap your current GraphQL handler with
the upload handler and you are good to go!

### Example implementation for [graph-gophers](https://github.com/graph-gophers/graphql-go)
#### Server
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	upload "github.com/jrkt/graphql-go-upload"
)

const schemaString = `
schema {
	query: Query
	mutation: Mutation
}

scalar Upload

type Query {}
type Mutation {
	uploadFiles(files: [Upload!]!): Boolean!
}`

type rootResolver struct{}

func (r *rootResolver) UploadFiles(ctx context.Context, args struct{ Files []upload.Upload }) (bool, error) {
	// handle files
	return true, nil
}

func main() {

	// parse schema
	schema := graphql.MustParseSchema(schemaString, &rootResolver{})

	// initialize http.Handler for /query entry point
	handler := &relay.Handler{Schema: schema}

	// create router
	router := mux.NewRouter()
	router.Handle("/query", upload.Handler(handler))

	fmt.Println("serving http on :8000")
	err := http.ListenAndServe(":8000", router)
	if err != nil {
		log.Fatalln(err)
	}
}
```

#### Client
This works out of the box with the [File](https://developer.mozilla.org/en-US/docs/Web/API/File) type on the front-end.
```javascript
const onChange = (e) => {
    // upload e.target.files
}

<input type='file' onChange={onChange} />
```
