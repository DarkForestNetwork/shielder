// evtype declares the the different event types sent by shuttermint
package evtype

var (
	CheckIn             = "shielder.check-in"
	PolyCommitment      = "shielder.poly-commitment-registered"
	PolyEval            = "shielder.poly-eval-registered"
	BatchConfig         = "shielder.batch-config"
	DecryptionSignature = "shielder.decryption-signature"
	EonStarted          = "shielder.eon-started"
	Accusation          = "shielder.accusation-registered"
	Apology             = "shielder.apology-registered"
)
