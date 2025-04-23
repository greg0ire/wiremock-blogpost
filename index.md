# Wiremock + testcontainers + Algolia + Go = ❤️

When dealing with a SaaS like [Algolia](https://www.algolia.com/), testing can
be a hassle. Ideally, you should not "mock what you do not own". In other words,
you should not mock libraries such as the Algolia SDK, not just because it
might evolve in unforeseen ways, but also because writing unit tests for a
piece of code where the logic is dictated by something external to the code is
not a good idea: you would not be testing the part that has the most
complexity.

To take a concrete example, let's imagine you want to index documents in
Algolia. There is an end goal behind that, and the end goal is that it is
possible to search for these documents.

Ideally, you would have a Docker container running Algolia locally that would
be super fast at indexing and use the same code your production Algolia app
uses, but sadly that does not exist, and I'm not hopeful it ever will.

In a legacy service I worked on, we have a test Algolia app that we use for
integration tests. It worked great, but in the past years, Algolia introduced a
new cloud-based architecture, and with this architecture, an indexing task can
take a lot more time to be "published". As a result, using a test application
on the cloud-based architecture is not an option anymore, as it slows the test
suite down to a crawl. 🐌

On a new project, I decided to re-evaluate my options, and remembered a tool
that seems to be the next best thing for the job: [Wiremock](https://wiremock.org).

In this post, I will guide you through the process of setting Wiremock and
testcontainers to test Algolia's own [quickstart guide for
Golang](https://www.algolia.com/doc/libraries/go/v4/).

## Wiremock: a VCR for HTTP

Wiremock is a testing tool that comes with a so-called ["record and
playback"](https://wiremock.org/docs/record-playback) feature.

It means you can do this once in your local environment:

```
┌────────────┐          ┌────────────────────────────┐       ┌─────────┐
│            ├─────────►│                            ├──────►│         │
│Your service│          │ Wiremock in recording mode │       │ Algolia │
│            │◄─────────┤                            │◄──────┤         │
└────────────┘          └────────────────────────────┘       └─────────┘
```

In recording mode, you give Wiremock a URL to record, and it will store files
representing the requests you made, and the corresponding responses. With
Algolia, it can be quite long, especially if you [wait for
operations](https://www.algolia.com/doc/api-reference/api-methods/wait-task).
What happens in practice is that the SDK will use a polling mechanism to check
if your task is published. This will result in a lot of similarly looking files.
This is not very interesting to reproduce in your test, so I recommend simply
deleting files representing a negative response to the question: "are the
changes published yet?". Those typically contain a JSON field called `status`
set to `notPublished` in their body, like so:

```json
{"status": "notPublished", "pendingTask":false}
```
When the file is published, this becomes:

```json
{"status": "published", "pendingTask":false}
```

The files have names that are
a bit ugly, so I usually rename them for clarity.
For example, you might rename
`1_indexes_test-index_task_226434943725-6e8689fa-9bbb-43fb-9d24-6824c02fc7d5.json`
to `index_test_task_published.json`.

Once your recording is done, you can run your tests like this:

```
┌────────────┐          ┌───────────────────────────┐
│            ├─────────►│                           │
│Your service│          │ Wiremock in playback mode │
│            │◄─────────┤                           │
└────────────┘          └───────────────────────────┘
```

In playback mode, Wiremock will respond to your request with the mappings it
has stored previously, and pretend to be Algolia. 🥸

While this does not shield you against breaking changes in the Algolia HTTP
API, it does come with a few advantages:

1. It shields you against breaking changes or bugs in the Algolia SDK.
2. You no longer have to mock the SDK, which is a bad practice and a pain to do.
A consequence of that is that your tests become easier to understand, and more
expressive, and that they check things at a higher level rather than focusing
on implementation details.
3. It still means that at least once, you do run the tests against the real
   thing, so if there is some issue that can only be detected at runtime, you
   will know about it.

Wiremock is a java application, but that shouldn't matter too much, especially
given there is an [official Docker
image](https://hub.docker.com/r/wiremock/wiremock) you can use.

## Testcontainers: Docker for your tests

At ManoMano, we use Gitlab CI. While it is possible to define [a Gitlab CI
service](https://docs.gitlab.com/ci/services/) with the aforementioned Docker image,
that's not a great solution because Gitlab services do not expose the full
power of Docker. For instance, mounting a volume is not possible, probably not
without heavy involvement of privileged users.

A great alternative is [testcontainers](https://testcontainers.com) +
[testcontainers Cloud](https://testcontainers.com/cloud/). Testcontainers is a
library available in many languages that allows you to start and stop Docker
containers during your tests, making it possible to get good isolation between
 tests.
Testcontainers Cloud is a service that allows you to run said containers
on a remote infrastructure, as opposed to running them on your own infrastructure,
which, if you want to use kubernetes runners for Gitlab, implies using Docker
in Docker, which is not great from the security standpoint.
Locally, you would still use a local docker container, but in the CI,
tescontainers will send requests to testcontainers cloud, to start and stop
containers. Enough unpaid endorsement, let's get to the code.

## Demo time

Let us follow [Algolia's quickstart guide for
Golang](https://www.algolia.com/doc/libraries/go/v4/) and see how easy it is to
test.

If you want to follow along, install
[mise-en-place](https://mise.jdx.dev/getting-started.html) and let's go!

For the sake of brevity, I will not systematically show the entirety of a file
I edit in all snippets, however I have tried to create one commit per step in
[this Github repository](https://github.com/greg0ire/wiremock-blogpost/commits),
in case you would like to play with the code or simply read it in your own
editor.

### Installing Go

```console
$ mise use go@1.24
```

### Creating a new project

```console
$ go mod init algolia-wiremock-testcontainers
```

### Installing the Algolia SDK

```console
$ go get github.com/algolia/algoliasearch-client-go/v4
```

### Setting up the environment

At this point, you will need to set up a test Algolia application. Once you are
done, you should have an application ID and an API key.

Let us use an unversioned env file to store our credentials.

```toml
# mise.toml
[env]
_.file = ".env"
```

```shell
# .env
ALGOLIA_APP_ID=changeme
ALGOLIA_API_KEY=changeme
```
You will need to replace `ALGOLIA_APP_ID` and `ALGOLIA_API_KEY` with values
from [your account](https://dashboard.algolia.com/account/api-keys).

```shell
# .gitignore
/.env
```

### Writing the code to be tested

Let us take the code from Algolia's quickstart guide and split it into two files:

First, we have the code under test where the only changes are getting the
environment variables from the actual environment, and renaming packages and
functions.

```go
// indexer.go

package indexer

import (
	"os"

	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
)

func indexRecord() {
	// Get Algolia credentials from environment variables
	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	indexName := "test-index"

	record := map[string]any{
		"objectID": "object-1",
		"name":     "test record",
	}

	// Create a new Algolia client
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
```

To make it work, you will need to install the Algolia SDK:

```console
$ go get github.com/algolia/algoliasearch-client-go/v4
$ go mod tidy
```

That call to `WaitForTask` is what is going to take the most time, and a good
reason not to use a real Algolia instance in your test suite. That's what we
are going to try first though.

### Writing the test with a real Algolia instance

Let's start simple and write a first version of the test that talks directly to
Algolia:

```go
// indexer_test.go

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
		// Ensure the test index is deleted after the test completes
		_, err := client.DeleteIndex(client.NewApiDeleteIndexRequest(indexName))
		if err != nil {
			t.Fatalf("Failed to delete index: %v", err)
		}
	})

	// Call the function under test - this should index a record in Algolia
	indexRecord()

	// Verify the record was indexed by searching for it
	// This search should return record with "test" in their contents
	// The quickstart guide currently uses a more complex version of this
	searchResp, err := client.SearchSingleIndex(
		client.NewApiSearchSingleIndexRequest(indexName).WithSearchParams(
			search.SearchParamsObjectAsSearchParams(search.NewEmptySearchParamsObject().SetQuery("test")),
		),
	)

	if err != nil {
		panic(err)
	}

	// Assert that there are hits
	if len(searchResp.Hits) == 0 {
		t.Fatal("No hits found")
	}
}
```

aaaaand that doesn't work:

```
panic: The maximum number of retries exceeded. (50/50) [recovered]
        panic: The maximum number of retries exceeded. (50/50)

goroutine 7 [running]:
testing.tRunner.func1.2({0x800600, 0xc00028d640})
        /home/gregoire/.local/share/mise/installs/go/1.24.2/src/testing/testing.go:1734 +0x21c
testing.tRunner.func1()
        /home/gregoire/.local/share/mise/installs/go/1.24.2/src/testing/testing.go:1737 +0x35e
panic({0x800600?, 0xc00028d640?})
        /home/gregoire/.local/share/mise/installs/go/1.24.2/src/runtime/panic.go:792 +0x132
algolia-wiremock-testcontainers.indexRecord()
        /home/gregoire/Documents/blogging/wiremock/indexer.go:39 +0x166
algolia-wiremock-testcontainers.TestIndexRecord(0xc000198540)
        /home/gregoire/Documents/blogging/wiremock/indexer_test.go:40 +0x20a
testing.tRunner(0xc000198540, 0x8a6c10)
        /home/gregoire/.local/share/mise/installs/go/1.24.2/src/testing/testing.go:1792 +0xf4
created by testing.(*T).Run in goroutine 1
        /home/gregoire/.local/share/mise/installs/go/1.24.2/src/testing/testing.go:1851 +0x413
FAIL    algolia-wiremock-testcontainers 185.745s
FAIL
```

I have many applications on this instance, some of which are very busy, let us
patch that real quick:

```
// Wait until indexing is done
_, err = client.WaitForTask(
	indexName,
	saveResp.TaskID,
	search.WithMaxRetries(100),
)
```

Exactly the type of thing that unit tests will not catch.

After that, the test passes (but it takes between several seconds or several
minutes to run depending on how busy the instance on which the application is
running is). Great! Now, let's add a proxy in the middle, and record all this.

### Adding Wiremock in record mode 📼

We are using Docker, so if we want to obtain the so-called
"mapping files" wiremock will create, we need to mount a volume on our Docker
container, and mount it in the right location.

Let us add 2 new dependencies to our project:

We could interact with Wiremock by calling the REST API with the `net/http`
package, but as it turns out, there is a dedicated SDK for that, and it supports
recording since [this pull request I sent](https://github.com/wiremock/go-wiremock/pull/33),
published as of version 1.13.0.

```console
$ go get github.com/wiremock/go-wiremock@v1.13.0
```

Next, we will need a way to start and stop the Wiremock container, and for that
as well, there is a library:

```console
$ go get github.com/wiremock/wiremock-testcontainers-go@v1.0.0-alpha-11
```

Yes, this is alpha software 😬

Let us start the container, with a volume mounting `testdata` in the current
directory on `/home/wiremock/mappings` in the container. This is where Wiremock
will create json files.

```go
// indexer_test.go

ctx := context.Background() // for some reason wiremock doesn't like the testing context

absolutePath, err := os.Getwd()
if err != nil {
	t.Fatalf("Failed to get current working directory: %v", err)
}

// Start the Wiremock container,  using the testcontainers library
container, err := testcontainers_wiremock.RunContainerAndStopOnCleanup(
	ctx,
	t,
	testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
		hostConfig.Binds = []string{
			absolutePath + "/testdata:/home/wiremock/mappings",
		}
	}),
	testcontainers_wiremock.WithImage("wiremock/wiremock:3.12.1"),
)

if err != nil {
	t.Fatalf("Failed to create wiremock container: %v", err)
}

// The endpoint changes every time, so we need to obtain it at runtime
host, err := container.Endpoint(ctx, "")

if err != nil {
	t.Fatalf("Failed to get wiremock container endpoint: %v", err)
}
```

Next, we need to change how we instantiate the Algolia client, so that it calls
Wiremock instead of Algolia:

```go
algoliaClient, err := search.NewClientWithConfig(search.SearchConfiguration{
	Configuration: transport.Configuration{
		AppID:  appID,
		ApiKey: apiKey,
		Hosts: []transport.StatefulHost{
			transport.NewStatefulHost("http", host, func(k call.Kind) bool {
				return true
			}),
		},
	},
})

if err != nil {
	t.Fatalf("Failed to create client: %v", err)
}
```

Note that I have renamed the client to `algoliaClient` to avoid confusion with
the Algolia client and the Wiremock client.

Let us also refactor our `indexRecord()` function to take the client as an argument:

```go
// indexer.go

func indexRecord(client *search.APIClient) {
	indexName := "test-index"

	record := map[string]any{
		"objectID": "object-1",
		"name":     "test record",
	}

	saveResp, err := client.SaveObject(
		client.NewApiSaveObjectRequest(indexName, record),
	)

	if err != nil {
		panic(err)
	}

	_, err = client.WaitForTask(
		indexName,
		saveResp.TaskID,
		search.WithMaxRetries(100),
	)

	if err != nil {
		panic(err)
	}
}
```

Next, let's start the recording, and for that we need a client to call
Wiremock's administration API:

```go
// indexer_test.go

wiremockClient := wiremock.NewClient("http://" + host)
t.Cleanup(func() {
	err := wiremockClient.Reset()
	if err != nil {
		t.Fatalf("Failed to reset wiremock: %v", err)
	}
})

// Call Wiremock's administration API to start recording
err = wiremockClient.StartRecording(fmt.Sprintf(
	"https://%s-dsn.algolia.net",
	appID,
))

if err != nil {
	t.Fatalf("Failed to start recording: %v", err)
}

t.Cleanup(func() {
	// Call Wiremock's administration API to stop recording and dump the mappings
	err := wiremockClient.StopRecording()
	if err != nil {
		t.Fatalf("Failed to stop recording: %v", err)
	}
})

// code that interacts with Algolia must be after
```

Now, let's run our tests again, check our `testdata` directory, and see what's
new.


```console
$ ls -1 testdata
1_indexes_test-index-4c8f560d-010a-4db9-916c-f5c112481fc8.json
1_indexes_test-index-e766b4bf-6087-499b-975f-722c451f1d4a.json
1_indexes_test-index_query-e08439bf-fd60-4fac-8181-11b72f7a7a1c.json
1_indexes_test-index_settings-9de043d2-5803-40a0-b19b-49d0d2a17dde.json
1_indexes_test-index_task_228603771246-0004a7a6-3759-452c-8d0d-ea2a2f948ea1.json
1_indexes_test-index_task_228603771246-012f5d71-2645-496c-ba68-b85dcafbce51.json
1_indexes_test-index_task_228603771246-050f8624-f6f5-4815-96ed-03f319cdbda0.json
1_indexes_test-index_task_228603771246-05510684-a31f-41a6-96aa-d722a7527e87.json
1_indexes_test-index_task_228603771246-07651b21-23b0-45b4-9e13-386775c37432.json
1_indexes_test-index_task_228603771246-0bfcad3d-4d19-403f-9197-2fe6214beeb2.json
1_indexes_test-index_task_228603771246-0d450b99-1af0-4761-8d92-d7b92a68c702.json
1_indexes_test-index_task_228603771246-0dd66ddc-d7e2-4f70-8a3f-f5934ade7ac1.json
1_indexes_test-index_task_228603771246-1490e160-63e7-466d-9405-9437efd31c68.json
1_indexes_test-index_task_228603771246-1d25eb69-2a0f-448d-ae60-7075c380837d.json
1_indexes_test-index_task_228603771246-1f675d54-54bc-41cc-a3b4-f3dcc153cb0b.json
1_indexes_test-index_task_228603771246-1f829991-f442-45b6-9463-6ad064eb7576.json
1_indexes_test-index_task_228603771246-22837672-68b3-4684-a1f0-5bf19a8a3e8d.json
1_indexes_test-index_task_228603771246-25162e07-d196-4527-b476-173b7d62acf7.json
1_indexes_test-index_task_228603771246-26087497-e8ee-429a-a98e-a0100219c112.json
1_indexes_test-index_task_228603771246-264ce294-058a-482e-8d33-8be46740c7e9.json
1_indexes_test-index_task_228603771246-272cd045-d145-4b26-86b7-accad674a2db.json
1_indexes_test-index_task_228603771246-2ac3189e-9737-46df-a1b5-269e63a4d36a.json
1_indexes_test-index_task_228603771246-30a023df-b21b-43e7-b1a0-3f3c6ceec6d4.json
1_indexes_test-index_task_228603771246-315a9105-a169-4228-9d6a-7f20e9db3e4a.json
1_indexes_test-index_task_228603771246-3358c4a2-9c5d-4c7f-982f-1ca49c413b08.json
1_indexes_test-index_task_228603771246-34ef8259-1204-4026-9c1d-d43731a8d489.json
1_indexes_test-index_task_228603771246-37be5ee5-2f56-43a9-afe3-866859945d72.json
1_indexes_test-index_task_228603771246-3ba76a13-6052-4b9d-a7f2-61d3d364714d.json
1_indexes_test-index_task_228603771246-3be6df0a-3bcc-4104-81e0-28462875ed86.json
1_indexes_test-index_task_228603771246-3da6c29e-5603-4262-bfa8-37a06b021297.json
1_indexes_test-index_task_228603771246-3e321244-de3c-420e-978e-60b995839ca2.json
1_indexes_test-index_task_228603771246-41ed4f34-d05b-4e30-a9da-fd1b84d85aaa.json
1_indexes_test-index_task_228603771246-420e2d2c-2aa0-4afd-9cdd-5ac6ecc7bd20.json
1_indexes_test-index_task_228603771246-4220ae1a-447c-419f-86f9-6ee5bc40a121.json
1_indexes_test-index_task_228603771246-42b9612d-005a-4e5e-bb8c-e0e431790404.json
1_indexes_test-index_task_228603771246-450ab804-2980-478f-b1f2-c9cef5ba021c.json
1_indexes_test-index_task_228603771246-472b813d-f05a-4bca-bc14-c8f0b085388c.json
1_indexes_test-index_task_228603771246-4802a463-b49f-4ddf-83cd-49af7615c66a.json
1_indexes_test-index_task_228603771246-49ed408e-d531-4bc5-a711-2f1e4acc4ae2.json
1_indexes_test-index_task_228603771246-4a9c9104-f042-4f9a-a56b-dc4a9976bde7.json
1_indexes_test-index_task_228603771246-4ae60200-dbed-4c1b-878f-6dcf53cf4954.json
1_indexes_test-index_task_228603771246-4df81e83-7ce3-4ed1-9102-48c987a675de.json
1_indexes_test-index_task_228603771246-4e11ef64-ade6-40ca-b5c0-8a35684f73cd.json
1_indexes_test-index_task_228603771246-5383a9a9-7023-41f9-8092-09bbde387e7d.json
1_indexes_test-index_task_228603771246-55f8777f-8ff1-4fdb-ae35-86bd8d1641f6.json
1_indexes_test-index_task_228603771246-581cce11-3bbe-4492-b9b2-4bb4db47e50f.json
1_indexes_test-index_task_228603771246-58a94473-05f6-460d-b595-87d67a6389cc.json
1_indexes_test-index_task_228603771246-5aadb748-f27e-40b5-8166-33e61de59888.json
1_indexes_test-index_task_228603771246-5eabca6c-637d-4a3d-a883-9a0ac8d40449.json
1_indexes_test-index_task_228603771246-628fb430-b307-4257-8a23-becfcd8c1649.json
1_indexes_test-index_task_228603771246-660b5321-6c43-41e1-83d8-02f936a09bc9.json
1_indexes_test-index_task_228603771246-6b15a9eb-886f-43ff-af1e-2b82a8dacdf2.json
1_indexes_test-index_task_228603771246-6c610574-d0cf-4272-9260-cd5c42954b1c.json
1_indexes_test-index_task_228603771246-6e0d4b06-c3dc-47b8-858b-1d66ed65f708.json
1_indexes_test-index_task_228603771246-70303855-6eb1-465e-949b-40c85e6b5d2e.json
1_indexes_test-index_task_228603771246-712fc20e-8877-43e0-998b-3408c85a1645.json
1_indexes_test-index_task_228603771246-71ede81b-df18-4f39-97ea-757967a84599.json
1_indexes_test-index_task_228603771246-72c592d9-6d61-4a22-bf3e-3d2559b319a8.json
1_indexes_test-index_task_228603771246-72de2fd6-675c-42e6-85d0-21b3947b3fc1.json
1_indexes_test-index_task_228603771246-74ca6363-881f-49b8-8dac-efd6f5c8d449.json
1_indexes_test-index_task_228603771246-75f4709b-ef34-4eed-9d2e-002a3237a880.json
1_indexes_test-index_task_228603771246-773d2b22-6e45-4b0b-943b-b8e261c38d9b.json
1_indexes_test-index_task_228603771246-7accc591-6809-4483-9b39-578557dcabd3.json
1_indexes_test-index_task_228603771246-7bd2b559-9f16-4c57-92bb-0020763419b1.json
1_indexes_test-index_task_228603771246-837f6509-a8b8-4f0a-98fd-4f3b82349acc.json
1_indexes_test-index_task_228603771246-8875e9eb-07f3-4565-9e45-9db6b8c4a662.json
1_indexes_test-index_task_228603771246-8b464ab1-bef7-4975-972a-c472bfca9a90.json
1_indexes_test-index_task_228603771246-8c42ea24-0b42-4ed3-91a3-7cbce16828e0.json
1_indexes_test-index_task_228603771246-932fe93f-2121-4b1a-bedb-334a4518b986.json
1_indexes_test-index_task_228603771246-9697ecdf-4079-4af0-9df5-2d3f44f2b26c.json
1_indexes_test-index_task_228603771246-9bbbf58d-a4ea-47ba-af43-d7a455cc333a.json
1_indexes_test-index_task_228603771246-9d4fbd76-3f4c-4605-aa88-49d105bf718c.json
1_indexes_test-index_task_228603771246-9f126ad6-8a7a-4a37-80ef-7528c62f939d.json
1_indexes_test-index_task_228603771246-9f247f3d-9ba1-447f-a9d1-f61919a40d65.json
1_indexes_test-index_task_228603771246-a01ab516-b606-47b2-b4db-c1188fd9527a.json
1_indexes_test-index_task_228603771246-a057d6b6-00fe-40ea-aee8-bc0873bfe66d.json
1_indexes_test-index_task_228603771246-a460d1b7-6d5f-46b4-9196-a0a49c6c9e10.json
1_indexes_test-index_task_228603771246-a7c15221-1a8f-48b2-9c20-c90bdb6f0074.json
1_indexes_test-index_task_228603771246-a90bdd7d-7be0-428b-973f-1ba429c10f99.json
1_indexes_test-index_task_228603771246-b06c842b-102e-4ca9-99b2-38d838166480.json
1_indexes_test-index_task_228603771246-b2fbbe73-e3cb-49ee-a008-05a3eb66c93d.json
1_indexes_test-index_task_228603771246-b4a8ce95-12f3-4786-9dca-e989f86873ef.json
1_indexes_test-index_task_228603771246-baf4c6a3-ac78-44df-9e5d-8798e9dcf1ff.json
1_indexes_test-index_task_228603771246-bee44a4d-b090-4ebf-b2c7-3b85a0c92000.json
1_indexes_test-index_task_228603771246-c367963c-3aca-41dd-a570-c3135702c841.json
1_indexes_test-index_task_228603771246-c4f8d672-f2ed-4b37-ad51-04ceacf8f3e0.json
1_indexes_test-index_task_228603771246-cbf506c8-70c9-406a-afaa-7db6b7212345.json
1_indexes_test-index_task_228603771246-cc952870-85f4-4057-bb42-24940e3a9050.json
1_indexes_test-index_task_228603771246-d46e09e2-0634-40c6-b121-9c488000a698.json
1_indexes_test-index_task_228603771246-d85de2d1-2dbd-472f-b7d7-e288f833867c.json
1_indexes_test-index_task_228603771246-d998fa67-ef81-407d-9ced-4549b63be22e.json
1_indexes_test-index_task_228603771246-dbebd7ae-ef9a-41b4-9f67-8e8cb871522f.json
1_indexes_test-index_task_228603771246-dec87bcc-2c17-4f38-97dc-f00d3346a00b.json
1_indexes_test-index_task_228603771246-e1228862-9f69-4224-8f46-135b307edd5a.json
1_indexes_test-index_task_228603771246-e4ea7ed5-6f42-4eb7-a54c-f25609c54f3a.json
1_indexes_test-index_task_228603771246-e4fdbcf5-1941-44bd-87bb-8f7c9b0c4718.json
1_indexes_test-index_task_228603771246-ea16fd95-c2ca-474f-a4d2-e556c5bd158c.json
1_indexes_test-index_task_228603771246-f657c74c-0e47-4a6b-887d-e003ba7118c6.json
1_indexes_test-index_task_228603771246-fa7feea5-229a-49af-ac9d-f4754539eb55.json
1_indexes_test-index_task_228603771246-fb081872-590e-443a-8c8b-957ac58fa541.json
1_indexes_test-index_task_228603771246-ff880e8d-f1cf-43ad-9e21-dfe5dab7a3c7.json
```

… OK that is quite a lot of files. 😅 As mentioned earlier, a lot of them are
about polling.

Let's find the one that we should keep:

```console
$ grep -i published testdata/*task*

testdata/1_indexes_test-index_task_228603771246-0004a7a6-3759-452c-8d0d-ea2a2f948ea1.json:    "body" : "{\"status\":\"notPublished\",\"pendingTask\":false}",
testdata/1_indexes_test-index_task_228603771246-012f5d71-2645-496c-ba68-b85dcafbce51.json:    "body" : "{\"status\":\"notPublished\",\"pendingTask\":false}",
…
testdata/1_indexes_test-index_task_228603771246-3ba76a13-6052-4b9d-a7f2-61d3d364714d.json:    "body" : "{\"status\":\"published\",\"pendingTask\":false}",
…
testdata/1_indexes_test-index_task_228603771246-fb081872-590e-443a-8c8b-957ac58fa541.json:    "body" : "{\"status\":\"notPublished\",\"pendingTask\":false}",
testdata/1_indexes_test-index_task_228603771246-ff880e8d-f1cf-43ad-9e21-dfe5dab7a3c7.json:    "body" : "{\"status\":\"notPublished\",\"pendingTask\":false}",
```

After removing the files with `notPublished`, we are left with the following
mapping files:

```console
$ ls -1 testdata
1_indexes_test-index-4c8f560d-010a-4db9-916c-f5c112481fc8.json
1_indexes_test-index-e766b4bf-6087-499b-975f-722c451f1d4a.json
1_indexes_test-index_query-e08439bf-fd60-4fac-8181-11b72f7a7a1c.json
1_indexes_test-index_settings-9de043d2-5803-40a0-b19b-49d0d2a17dde.json
1_indexes_test-index_task_228603771246-3ba76a13-6052-4b9d-a7f2-61d3d364714d.json
```

### Switching to playback mode 📺

Now that we have our mapping files, we can switch to playback mode. Let us
introduce a constant to turn recording and Algolia debugging on and off:

```go
// indexer_test.go

const record = false

// …

if record {
	debug.Enable()

	err = wiremockClient.StartRecording(fmt.Sprintf(
		"https://%s-dsn.algolia.net",
		appID,
	))

	if err != nil {
		t.Fatalf("Failed to start recording: %v", err)
	}

	t.Cleanup(func() {
		err := wiremockClient.StopRecording()
		if err != nil {
			t.Fatalf("Failed to stop recording: %v", err)
		}
	})
}
```

Note that I also moved the call to `debug.Enable()` to the recording block,
when replaying the tests, we do not really need to clutter the output with
Algolia debug information.

And now the test fails, with a rather clear error: apparently deleting the
files was not enough, and we need to also edit the scenario name to outline
that this is no longer the 43rd attempt.

```
--- FAIL: TestIndexRecord (1.87s)
panic: API error [404]
                                                       Request was not matched
                                                       =======================

        -----------------------------------------------------------------------------------------------------------------------
        | Closest stub                                             | Request                                                  |
        -----------------------------------------------------------------------------------------------------------------------
                                                                   |
        1_indexes_test-index_task_226434943725                     |
                                                                   |
        GET                                                        | GET
        /1/indexes/test-index/task/226434943725                    | /1/indexes/test-index/task/226434943725
                                                                   |
        [Scenario                                                  | [Scenario                                           <<<<< Scenario does not match
        'scenario-1-1-indexes-test-index-task-226434943725'        | 'scenario-1-1-indexes-test-index-task-226434943725'
        state:                                                     | state: Started]
        scenario-1-1-indexes-test-index-task-226434943725-43]      |
                                                                   |
        -----------------------------------------------------------------------------------------------------------------------
         [recovered]
        panic: API error [404]
                                                       Request was not matched
                                                       =======================

        -----------------------------------------------------------------------------------------------------------------------
        | Closest stub                                             | Request                                                  |
        -----------------------------------------------------------------------------------------------------------------------
                                                                   |
        1_indexes_test-index_task_226434943725                     |
                                                                   |
        GET                                                        | GET
        /1/indexes/test-index/task/226434943725                    | /1/indexes/test-index/task/226434943725
                                                                   |
        [Scenario                                                  | [Scenario                                           <<<<< Scenario does not match
        'scenario-1-1-indexes-test-index-task-226434943725'        | 'scenario-1-1-indexes-test-index-task-226434943725'
        state:                                                     | state: Started]
        scenario-1-1-indexes-test-index-task-226434943725-43]      |
                                                                   |
        -----------------------------------------------------------------------------------------------------------------------
```

After dropping `"requiredScenarioState" : "scenario-1-1-indexes-test-index-task-226434943725-43",`
from the mapping file about polling, the test passes again, only this time, it
passes in under 2 seconds.
It is possible to mention which scenario a mapping belongs to, allowing to do
things like "On the first 2 calls respond A, and on the 3rd return B". Based on
that, it is possible to build a complex choreography of requests/responses,
fulfilling all sorts of requirements.

### Making it work in the CI

After pushing the code, I got a bad surprise: the test fails in the CI, with
the following message:

```
tc-wiremock.go:73: create container: container create: Error response from daemon: Invalid bind mount config: mount source "/builds/product-discovery/ms.indexer/internal/import/brandsuggestion/testdata" is forbidden by the allow list [/home /tmp] - update the bind mounts configuration and restart the agent to enable
```

It would seem that we cannot use a bind mount in the CI. Let us use our
`record` constant to make the container options conditional:

```go
// indexer_test.go

if record {
	// Use a bind mount
	opts = append(opts, testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
		hostConfig.Binds = []string{
			absolutePath + "/testdata:/home/wiremock/mappings",
		}
	}))
} else {
	// Use a copy operation
	mappingFiles, err := os.ReadDir(absolutePath + "/testdata")
	if err != nil {
		t.Fatalf("Failed to read testdata directory: %v", err)
	}
	for _, mappingFile := range mappingFiles {
		opts = append(opts, testcontainers_wiremock.WithMappingFile(
			mappingFile.Name(),
			"testdata/"+mappingFile.Name(),
		))
	}
}
```

When recording, we mount the volume, which is not an issue because we are not
in the CI.
Otherwise, we use the `WithMappingFile` function which relies on a copy operation.
[That function][with-mapping-file-definition] is provided by the
`wiremock-testcontainers-go` library, which abstracts away the low-level
testcontainers API so that we can think in terms of mapping files rather than
just JSON files.

[with-mapping-file-definition]: https://pkg.go.dev/github.com/wiremock/wiremock-testcontainers-go#WithMappingFile


Not super satisfying, but it works.

## Wrapping up

The test is a bit long now, but some parts look generic and reusable. Let us
extract them to helpers.

```go
// indexer_test.go

package indexer

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/algolia/algoliasearch-client-go/v4/algolia/call"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/debug"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/transport"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"

	"github.com/wiremock/go-wiremock"
	testcontainers_wiremock "github.com/wiremock/wiremock-testcontainers-go"
)

const record = false

func spinUpContainer(t *testing.T) string {
	t.Helper()

	ctx := context.Background()

	absolutePath, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	var opts []testcontainers.ContainerCustomizer

	opts = append(opts, testcontainers_wiremock.WithImage("wiremock/wiremock:3.12.1"))

	if record {
		opts = append(opts, testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
			hostConfig.Binds = []string{
				absolutePath + "/testdata:/home/wiremock/mappings",
			}
		}))
	} else {
		mappingFiles, err := os.ReadDir(absolutePath + "/testdata")
		if err != nil {
			t.Fatalf("Failed to read testdata directory: %v", err)
		}
		for _, mappingFile := range mappingFiles {
			opts = append(opts, testcontainers_wiremock.WithMappingFile(
				mappingFile.Name(),
				"testdata/"+mappingFile.Name(),
			))
		}
	}

	container, err := testcontainers_wiremock.RunContainerAndStopOnCleanup(
		ctx,
		t,
		opts...,
	)

	if err != nil {
		t.Fatalf("Failed to create wiremock container: %v", err)
	}

	host, err := container.Endpoint(ctx, "")

	if err != nil {
		t.Fatalf("Failed to get wiremock container endpoint: %v", err)
	}

	return host
}

func startRecording(t *testing.T, host string, appID string) {
	t.Helper()

	wiremockClient := wiremock.NewClient("http://" + host)
	t.Cleanup(func() {
		err := wiremockClient.Reset()
		if err != nil {
			t.Fatalf("Failed to reset wiremock: %v", err)
		}
	})

	if !record {
		return
	}
	debug.Enable()
	err := wiremockClient.StartRecording(fmt.Sprintf(
		"https://%s-dsn.algolia.net",
		appID,
	))

	if err != nil {
		t.Fatalf("Failed to start recording: %v", err)
	}

	t.Cleanup(func() {
		err := wiremockClient.StopRecording()
		if err != nil {
			t.Fatalf("Failed to stop recording: %v", err)
		}
	})
}

func newTestClient(t *testing.T, host, appID, apiKey string) *search.APIClient {
	t.Helper()

	algoliaClient, err := search.NewClientWithConfig(search.SearchConfiguration{
		Configuration: transport.Configuration{
			AppID:  appID,
			ApiKey: apiKey,
			Hosts: []transport.StatefulHost{
				transport.NewStatefulHost("http", host, func(k call.Kind) bool {
					return true
				}),
			},
		},
	})

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return algoliaClient
}

func TestIndexRecord(t *testing.T) {

	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	indexName := "test-index"

	host := spinUpContainer(t)

	startRecording(t, host, appID)

	algoliaClient := newTestClient(t, host, appID, apiKey)

	_, err := algoliaClient.SetSettings(algoliaClient.NewApiSetSettingsRequest(
		"test-index",
		search.NewEmptyIndexSettings().SetSearchableAttributes([]string{"name"}),
	))

	if err != nil {
		t.Fatalf("Failed to set settings: %v", err)
	}

	t.Cleanup(func() {
		_, err := algoliaClient.DeleteIndex(algoliaClient.NewApiDeleteIndexRequest(indexName))
		if err != nil {
			t.Fatalf("Failed to delete index: %v", err)
		}
	})

	indexRecord(algoliaClient)

	searchResp, err := algoliaClient.SearchSingleIndex(
		algoliaClient.NewApiSearchSingleIndexRequest(indexName).WithSearchParams(
			search.SearchParamsObjectAsSearchParams(search.NewEmptySearchParamsObject().SetQuery("test")),
		),
	)

	if err != nil {
		panic(err)
	}

	if len(searchResp.Hits) == 0 {
		t.Fatal("No hits found")
	}

	firstHit := searchResp.Hits[0]

	// Assert that the first hit has the expected name
	if firstHit.AdditionalProperties["name"] != "test record" {
		t.Fatalf("Expected name to be 'test record', got '%s'", firstHit.AdditionalProperties["name"])
	}
}
```

And now our test fits on a single screen 🙂
I also added an extra assertion just to be sure we get the expected record, and
that's OK, since it does not mean extra calls to Algolia.
Now that we have paid the cost of writing that first step, writing more tests
should be easier, and bring a lot of value to the project.
