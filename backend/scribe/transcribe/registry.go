package transcribe

// DefaultProvider returns the provider the pipeline uses when the user
// hasn't explicitly chosen one. Today that's always LocalWhisperCpp;
// once cloud settings land we pick based on the persisted config.
func DefaultProvider() Provider { return LocalWhisperCpp{} }
