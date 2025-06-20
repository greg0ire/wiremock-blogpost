package indexer

import (
	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
)

func indexRecord(client *search.APIClient) { // nolint:unused // we do not want to write a full-fleged app
	indexName := "test-index"

	record := map[string]any{
		"objectID": "object-1",
		"name":     "test record",
	}

	// Add record to an index
	saveResp, err := client.SaveObject(
		client.NewApiSaveObjectRequest(indexName, record),
	)

	if err != nil {
		panic(err)
	}

	// Wait until indexing is done
	_, err = client.WaitForTask(
		indexName,
		saveResp.TaskID,
		search.WithMaxRetries(100),
	)

	if err != nil {
		panic(err)
	}
}
