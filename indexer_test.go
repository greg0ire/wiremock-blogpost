package indexer

import (
	"os"
	"testing"

	"github.com/algolia/algoliasearch-client-go/v4/algolia/debug"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
)

func TestIndexRecord(t *testing.T) {
	debug.Enable() // helps with seeing the progress, since this is super long

	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	indexName := "test-index"

	client, err := search.NewClient(appID, apiKey)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create index indirectly by setting settings
	_, err = client.SetSettings(client.NewApiSetSettingsRequest(
		"test-index",
		search.NewEmptyIndexSettings().SetSearchableAttributes([]string{"name"}),
	))

	if err != nil {
		t.Fatalf("Failed to set settings: %v", err)
	}

	t.Cleanup(func() {
		_, err := client.DeleteIndex(client.NewApiDeleteIndexRequest(indexName))
		if err != nil {
			t.Fatalf("Failed to delete index: %v", err)
		}
	})

	indexRecord()

	// Search for 'test'
	searchResp, err := client.SearchSingleIndex(
		client.NewApiSearchSingleIndexRequest(indexName).WithSearchParams(
			search.SearchParamsObjectAsSearchParams(search.NewEmptySearchParamsObject().SetQuery("test")),
		),
	)

	if err != nil {
		panic(err)
	}

	if len(searchResp.Hits) == 0 {
		t.Fatal("No hits found")
	}
}
