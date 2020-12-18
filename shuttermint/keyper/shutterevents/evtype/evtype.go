// evtype declares the the different event types sent by shuttermint
package evtype

var (
	Accusation          = "shielder.accusation-registered"
	Apology             = "shielder.apology-registered"
	BatchConfig         = "shielder.batch-config"
	CheckIn             = "shielder.check-in"
	DecryptionSignature = "shielder.decryption-signature"
	EonStarted          = "shielder.eon-started"
	PolyCommitment      = "shielder.poly-commitment-registered"
	PolyEval            = "shielder.poly-eval-registered"
)
