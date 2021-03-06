// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tsdb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

// Convert a SeriesSet into a form useable with reflect.DeepEqual.
func readSeriesSet(ss SeriesSet) (map[string][]sample, error) {
	result := map[string][]sample{}

	for ss.Next() {
		series := ss.At()

		samples := []sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, sample{t: t, v: v})
		}

		name := series.Labels().String()
		result[name] = samples
		if err := ss.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	defer querier.Close()
	seriesSet, err := readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{})

	err = app.Commit()
	require.NoError(t, err)

	querier = db.Querier(0, 1)
	defer querier.Close()

	seriesSet, err = readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{`{foo="bar"}`: []sample{{t: 0, v: 0}}})
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	if err != nil {
		t.Fatalf("Error opening database: %q", err)
	}
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	err = app.Rollback()
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	defer querier.Close()

	seriesSet, err := readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{})
}

func TestDBAppenderAddRef(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	app := db.Appender()
	defer app.Rollback()

	ref, err := app.Add(labels.FromStrings("a", "b"), 0, 0)
	require.NoError(t, err)

	// Head sequence number should be in 3rd MSB and be greater than 0.
	gen := (ref << 16) >> 56
	require.True(t, gen > 1)

	// Reference must be valid to add another sample.
	err = app.AddFast(ref, 1, 1)
	require.NoError(t, err)

	// AddFast for the same timestamp must fail if the generation in the reference
	// doesn't add up.
	refBad := ref | ((gen + 1) << 4)
	err = app.AddFast(refBad, 1, 1)
	require.Error(t, err)

	require.Equal(t, 2, app.(*dbAppender).samples)
}
