package observe

// World describes the observable outside world, i.e. the shielder and main chain instance
type World struct {
	Shielder   *Shielder
	MainChain *MainChain
}
