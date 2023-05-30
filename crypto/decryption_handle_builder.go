package crypto

// DecryptionHandleBuilder allow to configure a decryption handle
// to decrypt a pgp message.
type DecryptionHandleBuilder struct {
	handle       *decryptionHandle
	defaultClock Clock
	err          error
}

func newDecryptionHandleBuilder(clock Clock) *DecryptionHandleBuilder {
	return &DecryptionHandleBuilder{
		handle:       defaultDecryptionHandle(clock),
		defaultClock: clock,
	}
}

// DecryptionKeys sets the secret keys for decrypting the pgp message.
// Assumes that the the message was encrypted towards one of the secret keys.
// Triggers the hybrid decryption mode.
// If not set, set another field for the type of decryption: SessionKey or Password
func (dpb *DecryptionHandleBuilder) DecryptionKeys(decryptionKeyRing *KeyRing) *DecryptionHandleBuilder {
	dpb.handle.DecryptionKeyRing = decryptionKeyRing
	return dpb
}

func (dpb *DecryptionHandleBuilder) DecryptionKey(decryptionKey *Key) *DecryptionHandleBuilder {
	var err error
	if dpb.handle.DecryptionKeyRing == nil {
		dpb.handle.DecryptionKeyRing, err = NewKeyRing(decryptionKey)
	} else {
		err = dpb.handle.DecryptionKeyRing.AddKey(decryptionKey)
	}
	dpb.err = err
	return dpb
}

// SessionKey sets a session key for decrypting the pgp message.
// Assumes the the message was encrypted with session key provided.
// Triggers the session key decryption mode.
// If not set, set another field for the type of decryption: DecryptionKeys or Password
func (dpb *DecryptionHandleBuilder) SessionKey(sessionKey *SessionKey) *DecryptionHandleBuilder {
	dpb.handle.SessionKey = sessionKey
	return dpb
}

// Password sets a password that is used to derive a key to decrypt the pgp message.
// Assumes the the message was encrypted with a key derived from the password.
// Triggers the password decryption mode.
// If not set, set another field for the type of decryption: DecryptionKeys or SessionKey
func (dpb *DecryptionHandleBuilder) Password(password []byte) *DecryptionHandleBuilder {
	dpb.handle.Password = password
	return dpb
}

// VerifyKeys sets the public keys for verifying the signatures of the pgp message, if any.
// If not set, the signatures cannot be verified.
func (dpb *DecryptionHandleBuilder) VerifyKeys(verifyKeys *KeyRing) *DecryptionHandleBuilder {
	dpb.handle.VerifyKeyRing = verifyKeys
	return dpb
}

func (dpb *DecryptionHandleBuilder) VerifyKey(key *Key) *DecryptionHandleBuilder {
	var err error
	if dpb.handle.VerifyKeyRing == nil {
		dpb.handle.VerifyKeyRing, err = NewKeyRing(key)
	} else {
		err = dpb.handle.VerifyKeyRing.AddKey(key)
	}
	dpb.err = err
	return dpb
}

// VerificationContext sets a verification context for signatures of the pgp message, if any.
// Only considered if VerifyKeys are set.
func (dpb *DecryptionHandleBuilder) VerificationContext(verifyContext *VerificationContext) *DecryptionHandleBuilder {
	dpb.handle.VerificationContext = verifyContext
	return dpb
}

// VerifyTime sets the verification time to the provided timestamp.
// If not set, the systems current time is used for signature verification.
func (dpb *DecryptionHandleBuilder) VerifyTime(unixTime int64) *DecryptionHandleBuilder {
	dpb.handle.clock = NewConstantClock(unixTime)
	return dpb
}

// DisableVerifyTimeCheck disables the check for comparing the signature creation time
// against the verification time.
func (dpb *DecryptionHandleBuilder) DisableVerifyTimeCheck() *DecryptionHandleBuilder {
	dpb.handle.DisableVerifyTimeCheck = true
	return dpb
}

// DisableIntendedRecipients indicates if the signature verification should not check if
// the decryption key matches the intended recipients of the message.
// If disabled, the decryption methods throw no error in a non-matching case.
func (dpb *DecryptionHandleBuilder) DisableIntendedRecipients() *DecryptionHandleBuilder {
	dpb.handle.DisableIntendedRecipients = true
	return dpb
}

// RetrieveSessionKey sets the flag to indicate if the session key used for decryption
// should be returned to the caller of the decryption function.
func (dpb *DecryptionHandleBuilder) RetrieveSessionKey() *DecryptionHandleBuilder {
	dpb.handle.RetrieveSessionKey = true
	return dpb
}

// New creates a DecryptionHandle and checks that the given
// combination of parameters is valid. If one of the parameters are invalid
// the latest error is returned.
func (dpb *DecryptionHandleBuilder) New() (PGPDecryption, error) {
	if dpb.err != nil {
		return nil, dpb.err
	}
	dpb.err = dpb.handle.validate()
	if dpb.err != nil {
		return nil, dpb.err
	}
	handle := dpb.handle
	dpb.handle = defaultDecryptionHandle(dpb.defaultClock)
	return handle, nil
}

func (dpb *DecryptionHandleBuilder) Error() error {
	return dpb.err
}