package main

// Analyze disk usage of state.gob file

import (
	"encoding/gob"
	"fmt"
	"os"

	"shielder/shuttermint/keyper"
	"shielder/shuttermint/keyper/observe"
)

type storedState struct {
	State     *keyper.State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
}

type DummyWriter struct {
	Size int
}

func (w *DummyWriter) Write(p []byte) (n int, err error) {
	w.Size += len(p)
	return len(p), nil
}

func gobsize(st storedState) int {
	dw := DummyWriter{}
	enc := gob.NewEncoder(&dw)
	err := enc.Encode(&st)
	if err != nil {
		panic(err)
	}
	return dw.Size
}

func report(id string, full int, st storedState) {
	size := gobsize(st)
	percent := 100.0 * float64(size) / float64(full)
	fmt.Printf("%18s: %10d   %5.1f\n", id, size, percent)
}

func main() {
	gobpath := "state.gob"
	gobfile, err := os.Open(gobpath)
	if err != nil {
		panic(err)
	}
	dec := gob.NewDecoder(gobfile)
	st := storedState{}
	err = dec.Decode(&st)
	if err != nil {
		panic(err)
	}

	full := gobsize(st)
	report("full", full, st)
	report("state", full, storedState{State: st.State})

	report("main", full, storedState{MainChain: st.MainChain})
	report("shielder full", full, storedState{Shielder: st.Shielder})
	report("shielder batches", full, storedState{Shielder: &observe.Shielder{Batches: st.Shielder.Batches}})
	report("shielder eons", full, storedState{Shielder: &observe.Shielder{Eons: st.Shielder.Eons}})

	cl := st.Shielder.Clone()
	for i := 0; i < len(cl.Eons); i++ {
		d := &cl.Eons[i]
		d.EpochSecretKeyShares = nil
	}
	report("shielder no shares", full, storedState{Shielder: cl})
	if st.State.SyncHeight == 0 {
		st.State.SyncHeight = st.Shielder.CurrentBlock + 1
	}
	filter := st.State.GetShielderFilter(st.MainChain)
	fmt.Printf("FILTER: %+v\n", filter)
	report("filtered", full, storedState{Shielder: st.Shielder.ApplyFilter(filter)})
}
