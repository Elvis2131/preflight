package core

// DELIBERATE I1 VIOLATION, for demonstrating that CI catches it (PC-6). Never merged.
import "os"

var ciProofReadsEnvironment = os.Getenv("HOME")
