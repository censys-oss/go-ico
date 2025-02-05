package ico

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeAll(t *testing.T) {
	assert := assert.New(t)
	files, _ := filepath.Glob("testdata/favicons/*.ico")
	for _, f := range files {
		fmt.Println()
		fmt.Println("WORKING WITH", f)
		fmt.Println()
		icoData, err := os.ReadFile(f)
		assert.NoError(err, f)

		r := bytes.NewReader(icoData)
		ic, err := Decode(r)
		assert.NoError(err, f)

		for i, im := range ic {
			var jpgName string
			if len(ic) == 1 {
				jpgName = f + ".jpg"
			} else {
				jpgName = f + fmt.Sprintf("-%d.jpg", i)
			}
			jpgData, err := os.ReadFile(jpgName)
			assert.NoError(err, jpgName)

			r = bytes.NewReader(jpgData)
			jpgImage, err := jpeg.Decode(r)
			assert.NoError(err, jpgName)

			assert.Equal(im.Bounds(), jpgImage.Bounds())
		}
	}
}
