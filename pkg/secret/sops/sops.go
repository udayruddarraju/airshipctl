package sops

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"go.mozilla.org/sops/v3"
	"go.mozilla.org/sops/v3/aes"
	"go.mozilla.org/sops/v3/cmd/sops/common"
	"go.mozilla.org/sops/v3/keys"
	"go.mozilla.org/sops/v3/keyservice"
	"go.mozilla.org/sops/v3/pgp"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

type Client interface {
	InitializeKeys() error
	Encrypt(fromFile string, toFile string) ([]byte, error)
	Decrypt(fromFile string, toFile string) ([]byte, error)
}

type localGpg struct {
	clusterName      string
	clusterNamespace string
	kclient          client.Interface
	encryptionSecret *corev1.Secret
}

func NewClient(kclient client.Interface, clusterName string, clusterNamespace string) Client {
	return &localGpg{
		kclient:          kclient,
		clusterName:      clusterName,
		clusterNamespace: clusterNamespace,
	}
}

func (lg *localGpg) InitializeKeys() error {
	gpgSecretName := fmt.Sprintf("%s-gpg-encryption-key", lg.clusterName)
	secret, err := lg.getSecretFromApi(gpgSecretName, lg.clusterNamespace)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		// generate key pair and save it as secret
		pubKeyBytes, privKeyBytes, err := lg.GenerateGpgKeyPair(lg.clusterName)
		if err != nil {
			return err
		}
		secret, err = lg.createGpgSecret(gpgSecretName, lg.clusterNamespace, pubKeyBytes, privKeyBytes)
		if err != nil {
			return err
		}
	}
	// import the key locally
	if err = lg.ImportGpgKeyPairLocally(secret, lg.clusterName); err != nil {
		return err
	}
	lg.encryptionSecret = secret
	return nil
}

func (lg *localGpg) ImportGpgKeyPairLocally(secret *corev1.Secret, clusterName string) error {
	tmpPriKeyFileName := fmt.Sprintf("/tmp/%s.pri", clusterName)

	if err := writeFile(tmpPriKeyFileName, secret.Data["pri_key"]); err != nil {
		return err
	}
	defer func() {
		os.Remove(tmpPriKeyFileName)
	}()

	gpgCmd := exec.Command("gpg", "--import", tmpPriKeyFileName)
	gpgCmd.Run()
	return nil
}

func (lg *localGpg) Encrypt(fromFile string, toFile string) ([]byte, error) {
	groups, err := lg.getKeyGroup(lg.encryptionSecret.Data["pub_key"])
	if err != nil {
		return nil, err
	}
	store := common.DefaultStoreForPath(fromFile)
	fileBytes, err := ioutil.ReadFile(fromFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %s", err)
	}

	branches, err := store.LoadPlainFile(fileBytes)
	if err != nil {
		return nil, err
	}

	if err := lg.ensureNoMetadata(branches[0]); err != nil {
		return nil, err
	}

	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups:      groups,
			Version:        "3.6.0",
			EncryptedRegex: "^data",
		},
		FilePath: fromFile,
	}

	keySvc := keyservice.NewLocalClient()
	dataKey, errors := tree.GenerateDataKeyWithKeyServices([]keyservice.KeyServiceClient{keySvc})
	if len(errors) > 0 {
		return nil, fmt.Errorf("%s", errors)
	}
	if err = common.EncryptTree(common.EncryptTreeOpts{
		Tree:    &tree,
		Cipher:  aes.NewCipher(),
		DataKey: dataKey,
	}); err != nil {
		return nil, err
	}

	dstStore := common.DefaultStoreForPath(toFile)
	output, err := dstStore.EmitEncryptedFile(tree)
	if err != nil {
		return nil, err
	}

	if len(toFile) > 0 {
		err = ioutil.WriteFile(toFile, output, 0644)
		if err != nil {
			return nil, err
		}
	}

	return output, nil
}

func (lg *localGpg) Decrypt(fromFile string, toFile string) ([]byte, error) {
	keySvc := keyservice.NewLocalClient()
	tree, err := common.LoadEncryptedFileWithBugFixes(common.GenericDecryptOpts{
		Cipher:      aes.NewCipher(),
		InputStore:  common.DefaultStoreForPath(fromFile),
		InputPath:   fromFile,
		KeyServices: []keyservice.KeyServiceClient{keySvc},
	})
	if err != nil {
		return nil, err
	}

	if _, err = common.DecryptTree(common.DecryptTreeOpts{
		Tree:        tree,
		KeyServices: []keyservice.KeyServiceClient{keySvc},
		Cipher:      aes.NewCipher(),
	}); err != nil {
		return nil, err
	}

	dstStore := common.DefaultStoreForPath(toFile)
	output, err := dstStore.EmitPlainFile(tree.Branches)
	if err != nil {
		return nil, err
	}

	if len(toFile) > 0 {
		if err = writeFile(toFile, output); err != nil {
			return nil, err
		}
	}

	return output, nil
}

// Config for generating keys.
type Config struct {
	packet.Config
	// Expiry is the duration that the generated key will be valid for.
	Expiry time.Duration
}

// Key represents an OpenPGP key.
type Key struct {
	openpgp.Entity
}

// Values from https://tools.ietf.org/html/rfc4880#section-9
const (
	md5    = 1
	sha1   = 2
	sha256 = 8
	sha384 = 9
	sha512 = 10
	sha224 = 11
)

func (lg *localGpg) GenerateGpgKeyPair(name string) ([]byte, []byte, error) {
	tmpDir := "/tmp"
	key, err := lg.createKey(name, name, fmt.Sprintf("%s@cluster.local", name), &Config{})
	if err != nil {
		return nil, nil, err
	}

	priKeyFilename := fmt.Sprintf("%s/%s.pri", tmpDir, name)
	privateKey, err := key.ArmorPrivate(&Config{})
	if err != nil {
		return nil, nil, err
	}

	_, err = os.Create(priKeyFilename)
	if err != nil {
		return nil, nil, err
	}
	err = writeFile(priKeyFilename, []byte(privateKey))
	if err != nil {
		return nil, nil, err
	}

	pubKeyFilename := fmt.Sprintf("%s/%s.pub", tmpDir, name)
	publicKey, err := key.Armor()
	if err != nil {
		return nil, nil, err
	}
	_, err = os.Create(pubKeyFilename)
	if err != nil {
		return nil, nil, err
	}
	err = writeFile(pubKeyFilename, []byte(publicKey))
	if err != nil {
		return nil, nil, err
	}

	return []byte(publicKey), []byte(privateKey), nil
}

func (lg *localGpg) createKey(name, comment, email string, config *Config) (*Key, error) {
	// Create the key
	key, err := openpgp.NewEntity(name, comment, email, &config.Config)
	if err != nil {
		return nil, err
	}

	// Set expiry and algorithms. Self-sign the identity.
	dur := uint32(config.Expiry.Seconds())
	for _, id := range key.Identities {
		id.SelfSignature.KeyLifetimeSecs = &dur

		id.SelfSignature.PreferredSymmetric = []uint8{
			uint8(packet.CipherAES256),
			uint8(packet.CipherAES192),
			uint8(packet.CipherAES128),
			uint8(packet.CipherCAST5),
			uint8(packet.Cipher3DES),
		}

		id.SelfSignature.PreferredHash = []uint8{
			sha256,
			sha1,
			sha384,
			sha512,
			sha224,
		}

		id.SelfSignature.PreferredCompression = []uint8{
			uint8(packet.CompressionZLIB),
			uint8(packet.CompressionZIP),
		}

		err := id.SelfSignature.SignUserId(id.UserId.Id, key.PrimaryKey, key.PrivateKey, &config.Config)
		if err != nil {
			return nil, err
		}
	}

	// Self-sign the Subkeys
	for _, subkey := range key.Subkeys {
		subkey.Sig.KeyLifetimeSecs = &dur
		err := subkey.Sig.SignKey(subkey.PublicKey, key.PrivateKey, &config.Config)
		if err != nil {
			return nil, err
		}
	}

	r := Key{*key}
	return &r, nil
}

func (lg *localGpg) getSecretFromApi(name string, namespace string) (*corev1.Secret, error) {
	return lg.kclient.ClientSet().CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
}

func (lg *localGpg) createGpgSecret(name string, namespace string, pubKey []byte, priKey []byte) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		Data: map[string][]byte{
			"pub_key": pubKey,
			"pri_key": priKey,
		},
	}
	secret, err := lg.kclient.ClientSet().CoreV1().Secrets("kube-system").Create(secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// Armor returns the public part of a key in armored format.
func (key *Key) Armor() (string, error) {
	buf := new(bytes.Buffer)
	armor, err := armor.Encode(buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", err
	}
	key.Serialize(armor)
	armor.Close()

	return buf.String(), nil
}

// ArmorPrivate returns the private part of a key in armored format.
//
// Note: if you want to protect the string against varous low-level attacks,
// you should look at https://github.com/stouset/go.secrets and
// https://github.com/worr/secstring and then re-implement this function.
func (key *Key) ArmorPrivate(config *Config) (string, error) {
	buf := new(bytes.Buffer)
	armor, err := armor.Encode(buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		return "", err
	}
	c := config.Config
	key.SerializePrivate(armor, &c)
	armor.Close()

	return buf.String(), nil
}

// A keyring is simply one (or more) keys in binary format.
func (key *Key) Keyring() []byte {
	buf := new(bytes.Buffer)
	key.Serialize(buf)
	return buf.Bytes()
}

// A secring is simply one (or more) keys in binary format.
func (key *Key) Secring(config *Config) []byte {
	buf := new(bytes.Buffer)
	c := config.Config
	key.SerializePrivate(buf, &c)
	return buf.Bytes()
}

func (lg *localGpg) getKeyGroup(publicKeyBytes []byte) ([]sops.KeyGroup, error) {
	b := bytes.NewReader(publicKeyBytes)
	bufferedReader := bufio.NewReader(b)
	entities, err := openpgp.ReadArmoredKeyRing(bufferedReader)
	if err != nil {
		return nil, err
	}
	fingerprint := fmt.Sprintf("%X", entities[0].PrimaryKey.Fingerprint[:])
	var pgpKeys []keys.MasterKey
	for _, k := range pgp.MasterKeysFromFingerprintString(fingerprint) {
		pgpKeys = append(pgpKeys, k)
	}

	var group sops.KeyGroup
	group = append(group, pgpKeys...)
	return []sops.KeyGroup{group}, nil
}

func (lg *localGpg) ensureNoMetadata(branch sops.TreeBranch) error {
	for _, b := range branch {
		if b.Key == "sops" {
			return fmt.Errorf("file already encrypted")
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	return ioutil.WriteFile(path, content, 0644)
}
