package tempo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gabor-boros/minutes/internal/pkg/client"
	"github.com/gabor-boros/minutes/internal/pkg/client/tempo"
	"github.com/gabor-boros/minutes/internal/pkg/worklog"
	"github.com/stretchr/testify/require"
)

func getDataType(data interface{}) (res string) {
	t := reflect.TypeOf(data)

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
		res += "*"
	}

	for t.Kind() == reflect.Slice {
		t = t.Elem()
		res += "[]"
	}

	return res + t.Name()
}

type mockServerOpts struct {
	Path         string
	Method       string
	StatusCode   int
	Username     string
	Password     string
	Token        string
	TokenHeader  string
	RequestData  interface{}
	ResponseData *[]tempo.FetchEntry
}

func mockServer(t *testing.T, e *mockServerOpts) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, e.Method, r.Method, "API call methods are not matching")
		require.Equal(t, e.Path, r.URL.Path, "API call URLs are not matching")

		if e.Username != "" && e.Password != "" {
			username, password, _ := r.BasicAuth()
			require.Equal(t, e.Username, username, "API call basic auth username mismatch")
			require.Equal(t, e.Password, password, "API call basic auth password mismatch")
		}

		if e.Token != "" {
			headerValue := r.Header.Get(e.TokenHeader)
			require.Equal(t, e.Token, headerValue, "API call auth token mismatch")
		}

		if e.RequestData != nil {
			var data interface{}

			switch dataType := getDataType(e.RequestData); dataType {
			case "*SearchParams":
				data = e.RequestData.(*tempo.SearchParams)
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					t.Fatal(err)
				}

				require.Equal(t, data, e.RequestData, "cannot find expected search param")
			case "*[]UploadEntry":
				// Although in tests we define upload entries as a list, in the
				// reality it is uploaded one by one.
				allEntries := e.RequestData.(*[]tempo.UploadEntry)
				data = tempo.UploadEntry{}
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					t.Fatal(err)
				}

				for i, entry := range *allEntries {
					if data == entry {
						break
					}

					if i == len(*allEntries) && data != entry {
						t.Fatal("cannot find expected upload entry")
					}
				}
			default:
				t.Fatalf("%s is not a known data type", dataType)
			}
		}

		if e.ResponseData != nil {
			err := json.NewEncoder(w).Encode(e.ResponseData)
			require.Nil(t, err, "cannot encode response data")
		}

		w.WriteHeader(e.StatusCode)
	}))
}

func newMockServer(t *testing.T, opts *mockServerOpts) *httptest.Server {
	mockServer := mockServer(t, opts)
	require.NotNil(t, mockServer, "cannot create mock server")
	return mockServer
}

func TestTempoClient_FetchEntries(t *testing.T) {
	start := time.Date(2021, 10, 2, 0, 0, 0, 0, time.Local)
	end := time.Date(2021, 10, 2, 23, 59, 59, 0, time.Local)

	clientUsername := "Thor"
	clientPassword := "The strongest Avenger"

	expectedEntries := &[]worklog.Entry{
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   time.Second * 3600,
			UnbillableDuration: 0,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with him again",
			Start:              start,
			BillableDuration:   time.Second * 1800,
			UnbillableDuration: time.Second * 1800,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I helped him to get back on track",
			Start:              start,
			BillableDuration:   0,
			UnbillableDuration: time.Second * 3600,
		},
	}

	mockServer := newMockServer(t, &mockServerOpts{
		Path:       tempo.PathWorklogSearch,
		Method:     http.MethodPost,
		StatusCode: http.StatusOK,
		Username:   clientUsername,
		Password:   clientPassword,
		RequestData: &tempo.SearchParams{
			From:   start.Format("2006-01-02"),
			To:     end.Format("2006-01-02"),
			Worker: "steve-rogers",
		},
		ResponseData: &[]tempo.FetchEntry{
			{
				ID:               123,
				StartDate:        start,
				BillableSeconds:  3600,
				TimeSpentSeconds: 3600,
				Comment:          "I met with The Winter Soldier",
				WorkerKey:        "steve-rogers",
				Issue: tempo.Issue{
					ID:         789,
					Key:        "CPT-2014",
					AccountKey: "My Awesome Company",
					ProjectID:  456,
					ProjectKey: "MARVEL",
					Summary:    "Meet with The Winter Soldier",
				},
			},
			{
				ID:               456,
				StartDate:        start,
				BillableSeconds:  1800,
				TimeSpentSeconds: 3600,
				Comment:          "I met with him again",
				WorkerKey:        "steve-rogers",
				Issue: tempo.Issue{
					ID:         789,
					Key:        "CPT-2014",
					AccountKey: "My Awesome Company",
					ProjectID:  456,
					ProjectKey: "MARVEL",
					Summary:    "Meet with The Winter Soldier",
				},
			},
			{
				ID:               789,
				StartDate:        start,
				BillableSeconds:  0,
				TimeSpentSeconds: 3600,
				Comment:          "I helped him to get back on track",
				WorkerKey:        "steve-rogers",
				Issue: tempo.Issue{
					ID:         789,
					Key:        "CPT-2014",
					AccountKey: "My Awesome Company",
					ProjectID:  456,
					ProjectKey: "MARVEL",
					Summary:    "Meet with The Winter Soldier",
				},
			},
		},
	})
	defer mockServer.Close()

	httpClientOpts := &client.HTTPClientOptions{
		HTTPClient: http.DefaultClient,
		BaseURL:    mockServer.URL,
		Username:   clientUsername,
		Password:   clientPassword,
	}

	tempoClient := tempo.NewClient(&tempo.ClientOpts{
		BaseClientOpts: client.BaseClientOpts{
			HTTPClientOptions: *httpClientOpts,
		},
	})

	entries, err := tempoClient.FetchEntries(context.Background(), &client.FetchOpts{
		User:  "steve-rogers",
		Start: start,
		End:   end,
	})

	require.Nil(t, err, "cannot fetch entries")
	require.Equal(t, expectedEntries, entries, "fetched entries are not matching")
}

func TestTempoClient_UploadEntries(t *testing.T) {
	start := time.Date(2021, 10, 2, 0, 0, 0, 0, time.Local)

	clientUsername := "Thor"
	clientPassword := "The strongest Avenger"

	uploadOpts := &client.UploadOpts{
		User: "steve-rogers",
	}

	entries := []worklog.Entry{
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   time.Second * 3600,
			UnbillableDuration: 0,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with him again",
			Start:              start,
			BillableDuration:   time.Second * 3600,
			UnbillableDuration: 0,
		},
	}

	var responseEntries []tempo.UploadEntry
	for _, entry := range entries {
		responseEntries = append(responseEntries, tempo.UploadEntry{
			Comment:               entry.Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entry.Task.ID,
			Started:               entry.Start.Local().Format("2006-01-02"),
			BillableSeconds:       int(entry.BillableDuration.Seconds()),
			TimeSpentSeconds:      int((entry.BillableDuration + entry.UnbillableDuration).Seconds()),
			Worker:                uploadOpts.User,
		})
	}

	mockServer := newMockServer(t, &mockServerOpts{
		Path:        tempo.PathWorklogCreate,
		Method:      http.MethodPost,
		StatusCode:  http.StatusOK,
		Username:    clientUsername,
		Password:    clientPassword,
		RequestData: &responseEntries,
	})
	defer mockServer.Close()

	httpClientOpts := &client.HTTPClientOptions{
		HTTPClient: http.DefaultClient,
		BaseURL:    mockServer.URL,
		Username:   clientUsername,
		Password:   clientPassword,
	}

	tempoClient := tempo.NewClient(&tempo.ClientOpts{
		BaseClientOpts: client.BaseClientOpts{
			HTTPClientOptions: *httpClientOpts,
		},
	})

	err := tempoClient.UploadEntries(context.Background(), entries, uploadOpts)

	require.Nil(t, err, "cannot fetch entries")
}

func TestTempoClient_UploadEntries_TreatDurationAsBilled(t *testing.T) {
	start := time.Date(2021, 10, 2, 0, 0, 0, 0, time.Local)

	clientUsername := "Thor"
	clientPassword := "The strongest Avenger"

	uploadOpts := &client.UploadOpts{
		User:                  "steve-rogers",
		TreatDurationAsBilled: true,
	}

	entries := []worklog.Entry{
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   0,
			UnbillableDuration: time.Second * 3600,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with him again",
			Start:              start,
			BillableDuration:   time.Second * 3600,
			UnbillableDuration: 0,
		},
	}

	var responseEntries []tempo.UploadEntry
	for _, entry := range entries {
		responseEntries = append(responseEntries, tempo.UploadEntry{
			Comment:               entry.Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entry.Task.ID,
			Started:               entry.Start.Local().Format("2006-01-02"),
			BillableSeconds:       int(entry.BillableDuration.Seconds()),
			TimeSpentSeconds:      int((entry.BillableDuration + entry.UnbillableDuration).Seconds()),
			Worker:                uploadOpts.User,
		})
	}

	mockServer := newMockServer(t, &mockServerOpts{
		Path:        tempo.PathWorklogCreate,
		Method:      http.MethodPost,
		StatusCode:  http.StatusOK,
		Username:    clientUsername,
		Password:    clientPassword,
		RequestData: &responseEntries,
	})
	defer mockServer.Close()

	httpClientOpts := &client.HTTPClientOptions{
		HTTPClient: http.DefaultClient,
		BaseURL:    mockServer.URL,
		Username:   clientUsername,
		Password:   clientPassword,
	}

	tempoClient := tempo.NewClient(&tempo.ClientOpts{
		BaseClientOpts: client.BaseClientOpts{
			HTTPClientOptions: *httpClientOpts,
		},
	})

	err := tempoClient.UploadEntries(context.Background(), entries, uploadOpts)

	require.Nil(t, err, "cannot fetch entries")
}

func TestTempoClient_UploadEntries_RoundToClosestMinute(t *testing.T) {
	start := time.Date(2021, 10, 2, 0, 0, 0, 0, time.Local)

	clientUsername := "Thor"
	clientPassword := "The strongest Avenger"

	uploadOpts := &client.UploadOpts{
		User:                 "steve-rogers",
		RoundToClosestMinute: true,
	}

	entries := []worklog.Entry{
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   0,
			UnbillableDuration: time.Second * 30,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   0,
			UnbillableDuration: time.Second * 29,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   time.Second * 30,
			UnbillableDuration: time.Second * 29,
		},
		{
			Client: worklog.IDNameField{
				ID:   "My Awesome Company",
				Name: "My Awesome Company",
			},
			Project: worklog.IDNameField{
				ID:   strconv.Itoa(456),
				Name: "MARVEL",
			},
			Task: worklog.IDNameField{
				ID:   strconv.Itoa(789),
				Name: "CPT-2014",
			},
			Summary:            "Meet with The Winter Soldier",
			Notes:              "I met with The Winter Soldier",
			Start:              start,
			BillableDuration:   time.Second * 29,
			UnbillableDuration: time.Second * 30,
		},
	}

	responseEntries := []tempo.UploadEntry{
		{
			Comment:               entries[0].Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entries[0].Task.ID,
			Started:               entries[0].Start.Local().Format("2006-01-02"),
			BillableSeconds:       60,
			TimeSpentSeconds:      60,
			Worker:                uploadOpts.User,
		},
		{
			Comment:               entries[1].Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entries[1].Task.ID,
			Started:               entries[1].Start.Local().Format("2006-01-02"),
			BillableSeconds:       0,
			TimeSpentSeconds:      0,
			Worker:                uploadOpts.User,
		},
		{
			Comment:               entries[2].Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entries[2].Task.ID,
			Started:               entries[2].Start.Local().Format("2006-01-02"),
			BillableSeconds:       1,
			TimeSpentSeconds:      60,
			Worker:                uploadOpts.User,
		},
		{
			Comment:               entries[3].Notes,
			IncludeNonWorkingDays: true,
			OriginTaskID:          entries[3].Task.ID,
			Started:               entries[3].Start.Local().Format("2006-01-02"),
			BillableSeconds:       0,
			TimeSpentSeconds:      60,
			Worker:                uploadOpts.User,
		},
	}

	mockServer := newMockServer(t, &mockServerOpts{
		Path:        tempo.PathWorklogCreate,
		Method:      http.MethodPost,
		StatusCode:  http.StatusOK,
		Username:    clientUsername,
		Password:    clientPassword,
		RequestData: &responseEntries,
	})
	defer mockServer.Close()

	httpClientOpts := &client.HTTPClientOptions{
		HTTPClient: http.DefaultClient,
		BaseURL:    mockServer.URL,
		Username:   clientUsername,
		Password:   clientPassword,
	}

	tempoClient := tempo.NewClient(&tempo.ClientOpts{
		BaseClientOpts: client.BaseClientOpts{
			HTTPClientOptions: *httpClientOpts,
		},
	})

	err := tempoClient.UploadEntries(context.Background(), entries, uploadOpts)

	require.Nil(t, err, "cannot fetch entries")
}
