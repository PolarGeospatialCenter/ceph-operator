package keyrings

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

var (
	CLIENT_ADMIN_KEYRING = Keyring{
		Entity: "client.admin",
		Caps: map[string]string{
			"mds": "allow",
			"mon": "allow *",
			"osd": "allow *",
		},
	}
	MON_KEYRING = Keyring{
		Entity: "mon.",
		Caps: map[string]string{
			"mon": "allow *",
		},
	}
)

type Keyring struct {
	Entity string
	Key    string
	Caps   map[string]string
}

func (k *Keyring) GetSecretName(cluster string) string {
	return fmt.Sprintf("ceph-%s-%s-keyring", cluster, strings.Trim(k.Entity, "."))
}

func (k *Keyring) GetSecret(cluster string) *corev1.Secret {
	secret := &corev1.Secret{}

	secret.Name = k.GetSecretName(cluster)

	keyringFilename := "keyring"
	secret.StringData = make(map[string]string)
	secret.StringData[keyringFilename] = k.CreateKeyring()

	secret.SetLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel:   cluster,
		cephv1alpha1.KeyringEntityLabel: k.Entity,
	})

	return secret
}

func (k Keyring) CreateKeyring() string {
	buf := bytes.NewBufferString("")

	buf.WriteString(fmt.Sprintf("[%s]\n", k.Entity))
	buf.WriteString(fmt.Sprintf("    key = %s\n", k.Key))
	for k, v := range k.Caps {
		buf.WriteString(fmt.Sprintf("    caps %s = \"%s\"\n", k, v))
	}

	return buf.String()
}

func (k *Keyring) GenerateKey() error {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	if err != nil {
		return fmt.Errorf("unable to read random data to create key: %v", err)
	}

	encodedKey, err := EncodeKey(key, time.Now())
	if err != nil {
		return fmt.Errorf("unable to encode key: %v", err)
	}

	k.Key = encodedKey

	return nil
}

func EncodeKey(key []byte, t time.Time) (string, error) {

	if len(key) != 16 {
		return "", fmt.Errorf("Key must be of length 16 bytes got %d bytes", len(key))
	}
	buf := new(bytes.Buffer)

	headerValues := []interface{}{int16(1), int32(t.Unix()), int32(t.Nanosecond()), int16(16)}

	for _, v := range headerValues {
		err := binary.Write(buf, binary.LittleEndian, v)
		if err != nil {
			return "", err
		}
	}

	n, err := buf.Write(key)

	if n != 16 || err != nil {
		return "", fmt.Errorf("error concatinating secrets: %v", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
