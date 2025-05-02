package indexer

import (
	"os"

	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
)

func indexRecord() { // nolint:unused // we do not want to write a full-fleged app
	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	indexName := "test-index"

	record := map[string]any{
		"objectID": "object-1",
		"name":     "test record",
	}

	client, err := search.NewClient(appID, apiKey)

	if err != nil {
		panic(err)
	}

	// Add record to an index
	saveResp, err := client.SaveObject(
		client.NewApiSaveObjectRequest(indexName, record),
	)

	if err != nil {
		panic(err)
	}

	// Wait until indexing is done
	_, err = client.WaitForTask(indexName, saveResp.TaskID)

	if err != nil {
		panic(err)
	}
}
