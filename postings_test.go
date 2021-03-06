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
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockPostings struct {
	next  func() bool
	seek  func(uint32) bool
	value func() uint32
	err   func() error
}

func (m *mockPostings) Next() bool         { return m.next() }
func (m *mockPostings) Seek(v uint32) bool { return m.seek(v) }
func (m *mockPostings) Value() uint32      { return m.value() }
func (m *mockPostings) Err() error         { return m.err() }

func expandPostings(p Postings) (res []uint32, err error) {
	for p.Next() {
		res = append(res, p.At())
	}
	return res, p.Err()
}

func TestIntersect(t *testing.T) {
	var cases = []struct {
		a, b []uint32
		res  []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{6, 7, 8, 9, 10},
			res: nil,
		},
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{4, 5, 6, 7, 8},
			res: []uint32{4, 5},
		},
		{
			a:   []uint32{1, 2, 3, 4, 9, 10},
			b:   []uint32{1, 4, 5, 6, 7, 8, 10, 11},
			res: []uint32{1, 4, 10},
		}, {
			a:   []uint32{1},
			b:   []uint32{0, 1},
			res: []uint32{1},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a)
		b := newListPostings(c.b)

		res, err := expandPostings(Intersect(a, b))
		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}
}

func TestMultiIntersect(t *testing.T) {
	var cases = []struct {
		a, b, c []uint32
		res     []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []uint32{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []uint32{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []uint32{2, 5, 6, 1001},
		},
	}

	for _, c := range cases {
		pa := newListPostings(c.a)
		pb := newListPostings(c.b)
		pc := newListPostings(c.c)

		res, err := expandPostings(Intersect(pa, pb, pc))

		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}
}

func BenchmarkIntersect(t *testing.B) {
	var a, b, c, d []uint32

	for i := 0; i < 10000000; i += 2 {
		a = append(a, uint32(i))
	}
	for i := 5000000; i < 5000100; i += 4 {
		b = append(b, uint32(i))
	}
	for i := 5090000; i < 5090600; i += 4 {
		b = append(b, uint32(i))
	}
	for i := 4990000; i < 5100000; i++ {
		c = append(c, uint32(i))
	}
	for i := 4000000; i < 6000000; i++ {
		d = append(d, uint32(i))
	}

	i1 := newListPostings(a)
	i2 := newListPostings(b)
	i3 := newListPostings(c)
	i4 := newListPostings(d)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		if _, err := expandPostings(Intersect(i1, i2, i3, i4)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMultiMerge(t *testing.T) {
	var cases = []struct {
		a, b, c []uint32
		res     []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []uint32{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []uint32{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 999, 1000, 1001, 1200},
		},
	}

	for _, c := range cases {
		i1 := newListPostings(c.a)
		i2 := newListPostings(c.b)
		i3 := newListPostings(c.c)

		res, err := expandPostings(Merge(i1, i2, i3))
		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}
}

func TestMergedPostings(t *testing.T) {
	var cases = []struct {
		a, b []uint32
		res  []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{6, 7, 8, 9, 10},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{4, 5, 6, 7, 8},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			a:   []uint32{1, 2, 3, 4, 9, 10},
			b:   []uint32{1, 4, 5, 6, 7, 8, 10, 11},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a)
		b := newListPostings(c.b)

		res, err := expandPostings(newMergedPostings(a, b))
		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}

}

func TestMergedPostingsSeek(t *testing.T) {
	var cases = []struct {
		a, b []uint32

		seek    uint32
		success bool
		res     []uint32
	}{
		{
			a: []uint32{1, 2, 3, 4, 5},
			b: []uint32{6, 7, 8, 9, 10},

			seek:    0,
			success: true,
			res:     []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []uint32{1, 2, 3, 4, 5},
			b: []uint32{6, 7, 8, 9, 10},

			seek:    2,
			success: true,
			res:     []uint32{2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []uint32{1, 2, 3, 4, 5},
			b: []uint32{4, 5, 6, 7, 8},

			seek:    9,
			success: false,
			res:     nil,
		},
		{
			a: []uint32{1, 2, 3, 4, 9, 10},
			b: []uint32{1, 4, 5, 6, 7, 8, 10, 11},

			seek:    10,
			success: true,
			res:     []uint32{10, 11},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a)
		b := newListPostings(c.b)

		p := newMergedPostings(a, b)

		require.Equal(t, c.success, p.Seek(c.seek))
		lst, err := expandPostings(p)
		require.NoError(t, err)
		require.Equal(t, c.res, lst)
	}

	return
}

func TestBigEndian(t *testing.T) {
	num := 1000
	// mock a list as postings
	ls := make([]uint32, num)
	ls[0] = 2
	for i := 1; i < num; i++ {
		ls[i] = ls[i-1] + uint32(rand.Int31n(25)) + 2
	}

	beLst := make([]byte, num*4)
	for i := 0; i < num; i++ {
		b := beLst[i*4 : i*4+4]
		binary.BigEndian.PutUint32(b, ls[i])
	}

	t.Run("Iteration", func(t *testing.T) {
		bep := newBigEndianPostings(beLst)
		for i := 0; i < num; i++ {
			require.True(t, bep.Next())
			require.Equal(t, ls[i], bep.At())
		}

		require.False(t, bep.Next())
		require.Nil(t, bep.Err())
	})

	t.Run("Seek", func(t *testing.T) {
		table := []struct {
			seek  uint32
			val   uint32
			found bool
		}{
			{
				ls[0] - 1, ls[0], true,
			},
			{
				ls[4], ls[4], true,
			},
			{
				ls[500] - 1, ls[500], true,
			},
			{
				ls[600] + 1, ls[601], true,
			},
			{
				ls[600] + 1, ls[602], true,
			},
			{
				ls[600] + 1, ls[603], true,
			},
			{
				ls[0], ls[604], true,
			},
			{
				ls[600], ls[605], true,
			},
			{
				ls[999], ls[999], true,
			},
			{
				ls[999] + 10, ls[999], false,
			},
		}

		bep := newBigEndianPostings(beLst)

		for _, v := range table {
			require.Equal(t, v.found, bep.Seek(v.seek))
			require.Equal(t, v.val, bep.At())
			require.Nil(t, bep.Err())
		}
	})
}
