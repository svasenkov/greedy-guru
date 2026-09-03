package cdp_test

import (
	"context"
	"testing"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
)

func benchCrystal() *crystal.Crystal {
	return &crystal.Crystal{
		ID: "stub", Kind: "cdp", Version: 1,
		Steps: []crystal.Step{{Op: "eval", Value: "true"}},
	}
}

func TestMedianInt64(t *testing.T) {
	if cdp.MedianInt64(nil) != 0 {
		t.Fatal("empty")
	}
	if cdp.MedianInt64([]int64{3, 1, 2}) != 2 {
		t.Fatal("odd")
	}
	if cdp.MinInt64([]int64{3, 1, 2}) != 1 || cdp.MaxInt64([]int64{3, 1, 2}) != 3 {
		t.Fatal("min/max")
	}
}

func TestBenchReportSeqAndPar(t *testing.T) {
	c := benchCrystal()
	opt := cdp.Options{BaseURL: "http://example.test"}
	sessions := []cdp.Caller{&stub{}, &stub{}}
	rep := cdp.Report(context.Background(), sessions, c, opt, []int{1, 2}, 2, []int{2}, 3)
	if !rep.OK {
		t.Fatal(rep.Error)
	}
	if rep.Clock != cdp.BenchClock || rep.Repeat != 3 {
		t.Fatalf("%+v", rep)
	}
	if len(rep.Seq) != 2 || rep.Seq[0].N != 1 || rep.Seq[1].N != 2 {
		t.Fatalf("%+v", rep.Seq)
	}
	if len(rep.Seq[0].Runs) != 3 || rep.Seq[0].Best > rep.Seq[0].Median || rep.Seq[0].Median > rep.Seq[0].Max {
		t.Fatalf("%+v", rep.Seq[0])
	}
	if rep.Par == nil || rep.Par.N != 2 || len(rep.Par.Runs) != 3 {
		t.Fatalf("%+v", rep.Par)
	}
	if len(rep.Waves) != 1 || rep.Waves[0].N != 2 || len(rep.Waves[0].Runs) != 3 {
		t.Fatalf("%+v", rep.Waves)
	}
}

func TestParseIntList(t *testing.T) {
	got, err := cdp.ParseIntList("1, 5,10")
	if err != nil || len(got) != 3 || got[1] != 5 {
		t.Fatalf("%v %v", got, err)
	}
	empty, err := cdp.ParseIntList("")
	if err != nil || empty != nil {
		t.Fatalf("%v %v", empty, err)
	}
}
