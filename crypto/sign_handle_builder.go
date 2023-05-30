package crypto

type SignHandleBuilder struct {
	handle       *signatureHandle
	defaultClock Clock
	err          error
}

func newSignHandleBuilder(profile SignProfile, clock Clock) *SignHandleBuilder {
	return &SignHandleBuilder{
		handle:       defaultSignatureHandle(profile, clock),
		defaultClock: clock,
	}
}

func (shb *SignHandleBuilder) SigningKey(key *Key) *SignHandleBuilder {
	var err error
	if shb.handle.SignKeyRing == nil {
		shb.handle.SignKeyRing, err = NewKeyRing(key)
	} else {
		err = shb.handle.SignKeyRing.AddKey(key)
	}
	shb.err = err
	return shb
}

// SigningKeys sets the signing keys that are used to create signature of the message.
func (shb *SignHandleBuilder) SigningKeys(signingKeys *KeyRing) *SignHandleBuilder {
	shb.handle.SignKeyRing = signingKeys
	return shb
}

// SigningContext provides a signing context for the signature in the message.
// Triggers that each signature includes the sining context.
func (shb *SignHandleBuilder) SigningContext(siningContext *SigningContext) *SignHandleBuilder {
	shb.handle.SignContext = siningContext
	return shb
}

// Detached indicates if a detached signature should be produced.
// The sign output will be a detached signature message without the data included.
func (shb *SignHandleBuilder) Detached() *SignHandleBuilder {
	shb.handle.Detached = true
	return shb
}

// Armor indicates that the produced output should be armored.
func (shb *SignHandleBuilder) Armor() *SignHandleBuilder {
	shb.handle.Armored = true
	return shb
}

// ArmorWithHeader indicates that the produced signature should be armored
// with the given version and comment as header.
// Note that this option only affects the method SignHandle.SigningWriter
// and the headers in SignHandle.SignCleartext
func (shb *SignHandleBuilder) ArmorWithHeader(version, comment string) *SignHandleBuilder {
	shb.handle.Armored = true
	if shb.handle.ArmorHeaders == nil {
		shb.handle.ArmorHeaders = make(map[string]string)
	}
	shb.handle.ArmorHeaders["Version"] = version
	shb.handle.ArmorHeaders["Comment"] = comment
	return shb
}

// UTF8 indicates if the plaintext should be signed with a text type
// signature. If set, the plaintext is signed after
// canonicalising the line endings.
func (shb *SignHandleBuilder) UTF8() *SignHandleBuilder {
	shb.handle.IsUTF8 = true
	return shb
}

// SignTime sets the internal clock to always return
// the supplied unix time for signing instead of the device time
func (shb *SignHandleBuilder) SignTime(unixTime int64) *SignHandleBuilder {
	shb.handle.clock = NewConstantClock(unixTime)
	return shb
}

// New creates a SignHandle and checks that the given
// combination of parameters is valid. If the parameters are invalid
// an error is returned.
func (shb *SignHandleBuilder) New() (PGPSign, error) {
	if shb.err != nil {
		return nil, shb.err
	}
	shb.err = shb.handle.validate()
	if shb.err != nil {
		return nil, shb.err
	}
	handle := shb.handle
	shb.handle = defaultSignatureHandle(shb.handle.profile, shb.defaultClock)
	return handle, nil
}

func (dpb *SignHandleBuilder) Error() error {
	return dpb.err
}