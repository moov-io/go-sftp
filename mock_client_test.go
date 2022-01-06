package go_sftp_test

import (
	"io/ioutil"
	"strings"
	"testing"

	sftp "github.com/moovfinancial/go-sftp"
	"github.com/stretchr/testify/require"
)

func TestMockClient(t *testing.T) {
	client := sftp.NewMockClient()
	require.NoError(t, client.Ping())
	defer require.NoError(t, client.Close())

	_, err := client.Open("/missing.txt")
	require.Error(t, err)

	err = client.Delete("/missing.txt")
	require.Error(t, err)

	body := ioutil.NopCloser(strings.NewReader("contents"))
	err = client.UploadFile("/exists.txt", body)
	require.NoError(t, err)

	paths, err := client.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	require.Equal(t, "exists.txt", paths[0])
}
