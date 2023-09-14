package crypto

import (
	"bytes"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/ProtonMail/gopenpgp/v3/constants"
	"github.com/ProtonMail/gopenpgp/v3/internal"
	"github.com/pkg/errors"
)

type pgpSplitReader struct {
	encMessage, encSignature Reader
}

// pgpSplitReader implements the PGPSplitReader interface

func (mw *pgpSplitReader) Read(b []byte) (int, error) {
	return mw.encMessage.Read(b)
}

func (mw *pgpSplitReader) Signature() Reader {
	return mw.encSignature
}

func NewPGPSplitReader(pgpMessage Reader, pgpEncryptedSignature Reader) *pgpSplitReader {
	return &pgpSplitReader{
		encMessage:   pgpMessage,
		encSignature: pgpEncryptedSignature,
	}
}

// decryptStream decrypts the stream either with the secret keys or a password
func (dh *decryptionHandle) decryptStream(encryptedMessage Reader) (plainMessage *VerifyDataReader, err error) {
	var entries openpgp.EntityList
	config := &packet.Config{
		CacheSessionKey: dh.RetrieveSessionKey,
	}
	if dh.DecryptionKeyRing != nil {
		entries = dh.DecryptionKeyRing.entities
		checkIntendedRecipients := !dh.DisableIntendedRecipients
		config.CheckIntendedRecipients = &checkIntendedRecipients
	}
	if dh.VerifyKeyRing != nil {
		entries = append(entries, dh.VerifyKeyRing.entities...)
	}
	verifyTime := dh.clock().Unix()

	config.Time = NewConstantClock(verifyTime)
	if dh.VerificationContext != nil {
		config.KnownNotations = map[string]bool{constants.SignatureContextName: true}
	}

	var messageDetails *openpgp.MessageDetails
	if dh.DecryptionKeyRing != nil {
		// Private key based decryption
		messageDetails, err = openpgp.ReadMessage(encryptedMessage, entries, nil, config)
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: decrypting message with private keys failed")
		}
	} else {
		// Password based decryption
		prompt := createPasswordPrompt(dh.Password)
		messageDetails, err = openpgp.ReadMessage(encryptedMessage, entries, prompt, config)
		if err != nil {
			// Parsing errors when reading the message are most likely caused by incorrect password, but we cannot know for sure
			return nil, errors.New("gopenpgp: error in reading password protected message: wrong password or malformed message")
		}
	}

	// Add utf8 sanitizer if signature has type packet.SigTypeText
	internalReader := messageDetails.UnverifiedBody
	if messageDetails.IsSigned &&
		len(messageDetails.SignatureCandidates) > 0 &&
		messageDetails.SignatureCandidates[len(messageDetails.SignatureCandidates)-1].SigType == packet.SigTypeText {
		// TODO: This currently assumes that only one type of signature
		// can be present.
		internalReader = internal.NewSanitizeReader(internalReader)
	}
	return &VerifyDataReader{
		messageDetails,
		internalReader,
		dh.VerifyKeyRing,
		config.Time().Unix(),
		dh.DisableVerifyTimeCheck,
		false,
		dh.VerificationContext,
	}, err
}

func (dh *decryptionHandle) decryptStreamWithSession(dataPacketReader Reader) (plainMessage *VerifyDataReader, err error) {
	messageDetails, verifyTime, err := dh.decryptStreamWithSessionAndParse(dataPacketReader)
	if err != nil {
		return nil, errors.Wrap(err, "gopenpgp: error in reading message")
	}

	// Add utf8 sanitizer if signature has type packet.SigTypeText
	internalReader := messageDetails.UnverifiedBody
	if messageDetails.IsSigned &&
		len(messageDetails.SignatureCandidates) > 0 &&
		messageDetails.SignatureCandidates[len(messageDetails.SignatureCandidates)-1].SigType == packet.SigTypeText {
		// TODO: This currently assumes that only one type of signature
		// can be present.
		internalReader = internal.NewSanitizeReader(internalReader)
	}
	return &VerifyDataReader{
		messageDetails,
		internalReader,
		dh.VerifyKeyRing,
		verifyTime,
		dh.DisableVerifyTimeCheck,
		false,
		dh.VerificationContext,
	}, err
}

func (dh *decryptionHandle) decryptStreamWithSessionAndParse(messageReader io.Reader) (*openpgp.MessageDetails, int64, error) {
	var keyring openpgp.EntityList
	var decrypted io.ReadCloser
	var selectedSessionKey *SessionKey
	var err error
	// Read symmetrically encrypted data packet
	for _, sessionKeyCandidate := range dh.SessionKeys {
		decrypted, err = decryptStreamWithSessionKey(sessionKeyCandidate, messageReader)
		if err == nil { // No error occurred
			selectedSessionKey = sessionKeyCandidate
			break
		}
	}
	if selectedSessionKey == nil {
		return nil, 0, errors.Wrap(err, "gopenpgp: unable to decrypt message with session key")
	}

	config := &packet.Config{
		Time: NewConstantClock(dh.clock().Unix()),
	}

	if dh.VerificationContext != nil {
		config.KnownNotations = map[string]bool{constants.SignatureContextName: true}
	}

	// Push decrypted packet as literal packet and use openpgp's reader
	if dh.VerifyKeyRing != nil {
		keyring = append(keyring, dh.VerifyKeyRing.entities...)
	}
	if dh.DecryptionKeyRing != nil {
		keyring = append(keyring, dh.DecryptionKeyRing.entities...)
	}
	checkIntendedRecipients := !dh.DisableIntendedRecipients
	checkPacketSequence := false
	config.CheckIntendedRecipients = &checkIntendedRecipients
	config.CheckPacketSequence = &checkPacketSequence
	md, err := openpgp.ReadMessage(decrypted, keyring, nil, config)
	if err != nil {
		return nil, 0, errors.Wrap(err, "gopenpgp: unable to decode symmetric packet")
	}
	md.SessionKey = selectedSessionKey.Key
	md.UnverifiedBody = checkReader{decrypted, md.UnverifiedBody}
	return md, config.Time().Unix(), nil
}

func decryptStreamWithSessionKey(sessionKey *SessionKey, messageReader io.Reader) (io.ReadCloser, error) {
	var decrypted io.ReadCloser
	// Read symmetrically encrypted data packet
Loop:
	for {
		packets := packet.NewReader(messageReader)
		p, err := packets.Next()
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: unable to read symmetric packet")
		}

		// Decrypt data packet
		switch p := p.(type) {
		case *packet.EncryptedKey, *packet.SymmetricKeyEncrypted:
			// Ignore potential key packets
			continue
		case *packet.SymmetricallyEncrypted, *packet.AEADEncrypted:
			var dc packet.CipherFunction
			if !sessionKey.v6 {
				dc, err = sessionKey.GetCipherFunc()
				if err != nil {
					return nil, errors.Wrap(err, "gopenpgp: unable to decrypt with session key")
				}
			}
			encryptedDataPacket, isDataPacket := p.(packet.EncryptedDataPacket)
			if !isDataPacket {
				return nil, errors.Wrap(err, "gopenpgp: unknown data packet")
			}
			decrypted, err = encryptedDataPacket.Decrypt(dc, sessionKey.Key)
			if err != nil {
				return nil, errors.Wrap(err, "gopenpgp: unable to decrypt symmetric packet")
			}
			break Loop
		default:
			return nil, errors.New("gopenpgp: invalid packet type")
		}
	}
	return decrypted, nil
}

func (dh *decryptionHandle) decryptStreamAndVerifyDetached(encryptedData, encryptedSignature Reader) (plainMessage *VerifyDataReader, err error) {
	verifyTime := dh.clock().Unix()
	var mdData *openpgp.MessageDetails
	var signature io.Reader
	// Decrypt both messages
	if len(dh.SessionKeys) > 0 {
		// Decrypt with session key.
		mdData, _, err = dh.decryptStreamWithSessionAndParse(encryptedData)
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: error in reading data message")
		}
		// Decrypting reader for the encrypted signature
		mdSig, _, err := dh.decryptStreamWithSessionAndParse(encryptedSignature)
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: error in reading detached signature message")
		}
		signature = mdSig.UnverifiedBody
	} else {
		// Password or private keys
		config := &packet.Config{
			CacheSessionKey: dh.RetrieveSessionKey,
			Time:            NewConstantClock(verifyTime),
		}
		var entries openpgp.EntityList
		if dh.DecryptionKeyRing != nil {
			entries = append(entries, dh.DecryptionKeyRing.entities...)
		}
		// Decrypting reader for the encrypted data
		prompt := createPasswordPrompt(dh.Password)
		mdData, err = openpgp.ReadMessage(encryptedData, entries, prompt, config)
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: error in reading data message")
		}
		// Decrypting reader for the encrypted signature
		prompt = createPasswordPrompt(dh.Password)
		checkPacketSequence := false
		config.CheckPacketSequence = &checkPacketSequence
		mdSig, err := openpgp.ReadMessage(encryptedSignature, entries, prompt, config)
		if err != nil {
			return nil, errors.Wrap(err, "gopenpgp: error in reading detached signature message")
		}
		signature = mdSig.UnverifiedBody
	}

	checkIntendedRecipients := !dh.DisableIntendedRecipients
	config := &packet.Config{
		CheckIntendedRecipients: &checkIntendedRecipients,
	}

	// Verifying reader that wraps the decryption readers to verify the signature
	sigVerifyReader, err := verifyingDetachedReader(
		mdData.UnverifiedBody,
		signature,
		dh.VerifyKeyRing,
		dh.VerificationContext,
		dh.DisableVerifyTimeCheck,
		config,
		NewConstantClock(verifyTime),
	)
	if err != nil {
		return nil, err
	}
	// Update message details with information from the data of the pgp message
	sigVerifyReader.details.LiteralData = mdData.LiteralData
	sigVerifyReader.details.SessionKey = mdData.SessionKey
	return sigVerifyReader, nil
}

func getSignaturePacket(sig []byte) (*packet.Signature, error) {
	p, err := packet.Read(bytes.NewReader(sig))
	if err != nil {
		return nil, err
	}
	sigPacket, ok := p.(*packet.Signature)
	if !ok {
		return nil, errors.Wrap(err, "gopenpgp: invalid signature packet")
	}
	return sigPacket, nil
}

func createPasswordPrompt(password []byte) func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
	if password == nil {
		return nil
	}
	firstTimeCalled := true
	return func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		if firstTimeCalled {
			firstTimeCalled = false
			return password, nil
		}
		// Re-prompt still occurs if SKESK pasrsing fails (i.e. when decrypted cipher algo is invalid).
		// For most (but not all) cases, inputting a wrong passwords is expected to trigger this error.
		return nil, errors.New("gopenpgp: wrong password in symmetric decryption")
	}
}
