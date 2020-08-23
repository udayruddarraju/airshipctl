package resetsatoken_test

import (
	"fmt"
	"testing"

	//  "opendev.org/airship/airshipctl/pkg/secret/resetsatoken"
	//	ktesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"opendev.org/airship/airshipctl/pkg/config"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	"opendev.org/airship/airshipctl/pkg/k8s/client/fake"
	resetsatoken "opendev.org/airship/airshipctl/pkg/secret/resetsatoken"
)

const secretYaml = `
apiVersion: v1
kind: Service
metadata:
  name: push
  namespace: default
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    name: push
`

func TestRotateToken(t *testing.T) {

	testClientFactory := func(_ *environment.AirshipCTLSettings) (client.Interface, error) {
		return fake.NewClient(fake.WithTypedObjects(createPodResource("testpod", ""), createSecretResource("testsecret", ""),
			createSecretResource("testsecret-1", ""), createSecretResource("testsecret-3", ""), createSecretResourceNonSa("testsecret-4", ""))), nil
	}

	testClientFactoryErr := func(_ *environment.AirshipCTLSettings) (client.Interface, error) {
		return nil, fmt.Errorf("Not a valid factory")
	}

	err := resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "", "testsecret")
	assert.Equal(t, false, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "", "testsecret")
	assert.Equal(t, false, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "", "")
	assert.Equal(t, false, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "test", "")
	assert.Equal(t, true, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "", "testsecret-4")
	assert.Equal(t, true, err != nil)
	fmt.Println(err.Error())
	assert.Equal(t, "testsecret-4 is not a Service Account Token", err.Error())
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "", "testsecret-5")
	assert.Equal(t, true, err != nil)
	assert.Equal(t, "secrets \"testsecret-5\" not found", err.Error())
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "test", "testsecret-2")
	assert.Equal(t, true, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "test-0", "test-0")
	assert.Equal(t, true, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactory, "test-2", "testsecret-2")
	assert.Equal(t, true, err != nil)
	err = resetsatoken.RotateToken(fakeTestSettings(), testClientFactoryErr, "", "")
	assert.Equal(t, true, err != nil)

	//	client := fake.NewClient(fake.WithTypedObjects(createPodResource("testpod", ""), createSecretResource("testsecret", ""), createSecretResource("testsecret-1", "")))

	//	err := resetsatoken.DeleteSecret(client, "testsecret", "")

	//	assert.Equal(t, false, err != nil)

	//	err = resetsatoken.DeletePod(client, "testsecret", "")

	//	assert.Equal(t, false, err != nil)

	//	clientSecret := fake.NewClient(fake.WithTypedObjects(createSecretResource("testsecret", "")))

}

func createPodResource(name string, ns string) *corev1.Pod {

	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: apiv1.PodSpec{
			Containers: []apiv1.Container{
				{
					Name:  name,
					Image: "test",
				},
			},
			Volumes: []apiv1.Volume{
				apiv1.Volume{
					Name: "testsecret",
					VolumeSource: apiv1.VolumeSource{
						Secret: &apiv1.SecretVolumeSource{
							SecretName: "testsecret",
						},
					},
				},
			},
		},
	}
}

func createSecretResource(name string, ns string) *corev1.Secret {

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},

		Type: "kubernetes.io/service-account-token",
		Data: map[string][]byte{
			"ca.crt":    []byte("certificate"),
			"namespace": []byte("namespace"),
			"token":     []byte("token"),
		},
	}
}

func createSecretResourceNonSa(name string, ns string) *corev1.Secret {

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},

		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			"tls.crt": []byte("certificate"),
			"tls.key": []byte("key"),
		},
	}
}

func fakeTestSettings() *environment.AirshipCTLSettings {
	return &environment.AirshipCTLSettings{
		Config: &config.Config{
			Clusters:  map[string]*config.ClusterPurpose{"testCluster": nil},
			AuthInfos: map[string]*config.AuthInfo{"testAuthInfo": nil},
			Contexts: map[string]*config.Context{
				"testContext": {Manifest: "testManifest"},
			},
			Manifests: map[string]*config.Manifest{
				"testManifest": {TargetPath: "fixturesPath"},
			},
			CurrentContext: "testContext",
		},
	}
}
