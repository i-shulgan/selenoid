//go:build s3
// +build s3

package upload

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aerokube/selenoid/event"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func init() {
	s3 := &S3Uploader{}
	flag.StringVar(&(s3.Endpoint), "s3-endpoint", "", "S3 endpoint URL")
	flag.StringVar(&(s3.Region), "s3-region", "", "S3 region")
	flag.StringVar(&(s3.AccessKey), "s3-access-key", "", "S3 access key")
	flag.StringVar(&(s3.SecretKey), "s3-secret-key", "", "S3 secret key")
	flag.StringVar(&(s3.BucketName), "s3-bucket-name", "", "S3 bucket name")
	flag.StringVar(&(s3.KeyPattern), "s3-key-pattern", "$fileName", "S3 bucket name")
	flag.BoolVar(&(s3.ReducedRedundancy), "s3-reduced-redundancy", false, "Use reduced redundancy storage class")
	flag.BoolVar(&(s3.KeepFiles), "s3-keep-files", false, "Do not remove uploaded files")
	flag.StringVar(&(s3.IncludeFiles), "s3-include-files", "", "Pattern used to match and include files")
	flag.StringVar(&(s3.ExcludeFiles), "s3-exclude-files", "", "Pattern used to match and exclude files")
	flag.BoolVar(&(s3.ForcePathStyle), "s3-force-path-style", false, "Force path-style addressing for file upload")
	AddUploader(s3)
}

type S3Uploader struct {
	Endpoint          string
	Region            string
	AccessKey         string
	SecretKey         string
	BucketName        string
	KeyPattern        string
	ReducedRedundancy bool
	KeepFiles         bool
	IncludeFiles      string
	ExcludeFiles      string
	ForcePathStyle    bool

	client *s3.Client
}

func (s3u *S3Uploader) Init() {
	if s3u.Endpoint != "" {
		opts := []func(*awsconfig.LoadOptions) error{
			awsconfig.WithRegion(s3u.Region),
			// S3-compatible storages often reject the default trailing checksums.
			awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
			awsconfig.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
		}
		if s3u.AccessKey != "" && s3u.SecretKey != "" {
			opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s3u.AccessKey, s3u.SecretKey, "")))
		}
		cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
		if err != nil {
			log.Fatalf("[-] [INIT] [Failed to initialize S3 support: %v]", err)
		}
		s3u.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3u.Endpoint)
			o.UsePathStyle = s3u.ForcePathStyle
		})
		log.Printf("[-] [INIT] [Initialized S3 support: endpoint = %s, region = %s, bucketName = %s, accessKey = %s, keyPattern = %s, includeFiles = %s, excludeFiles = %s, forcePathStyle = %t]", s3u.Endpoint, s3u.Region, s3u.BucketName, s3u.AccessKey, s3u.KeyPattern, s3u.IncludeFiles, s3u.ExcludeFiles, s3u.ForcePathStyle)
	}
}

func (s3u *S3Uploader) Upload(createdFile event.CreatedFile) (bool, error) {
	if s3u.client != nil {
		filename := createdFile.Name
		fileMatches, err := FileMatches(s3u.IncludeFiles, s3u.ExcludeFiles, filename)
		if err != nil {
			return false, fmt.Errorf("invalid pattern: %v", err)
		}
		if !fileMatches {
			log.Printf("[%d] [SKIPPING_FILE] [%s] [Does not match specified patterns]", createdFile.RequestId, createdFile.Name)
			return false, nil
		}
		key := GetS3Key(s3u.KeyPattern, createdFile)
		file, err := os.Open(filename)
		if err != nil {
			return false, fmt.Errorf("failed to open file %s: %v", filename, err)
		}
		defer file.Close()
		uploadInput := &s3.PutObjectInput{
			Bucket: aws.String(s3u.BucketName),
			Key:    aws.String(key),
			Body:   file,
		}
		contentType := mime.TypeByExtension(filepath.Ext(filename))
		if contentType != "" {
			uploadInput.ContentType = aws.String(contentType)
		}
		if s3u.ReducedRedundancy {
			uploadInput.StorageClass = types.StorageClassReducedRedundancy
		}
		_, err = s3u.client.PutObject(context.Background(), uploadInput)
		if err != nil {
			return false, fmt.Errorf("failed to S3 upload %s as %s: %v", filename, key, err)
		}
		if !s3u.KeepFiles {
			err := os.Remove(filename)
			if err != nil {
				return true, fmt.Errorf("failed to remove uploaded file %s: %v", filename, err)
			}
		}
		return true, nil
	}
	return false, errors.New("S3 uploader is not initialized")
}

func FileMatches(includedFiles string, excludedFiles string, filename string) (bool, error) {
	fileIncluded := true
	if includedFiles != "" {
		fi, err := filepath.Match(includedFiles, filepath.Base(filename))
		if err != nil {
			return false, fmt.Errorf("failed to match included file: %v", err)
		}
		fileIncluded = fi
	}
	fileExcluded := false
	if excludedFiles != "" {
		fe, err := filepath.Match(excludedFiles, filepath.Base(filename))
		if err != nil {
			return false, fmt.Errorf("failed to match excluded file: %v", err)
		}
		fileExcluded = fe
	}
	return fileIncluded && !fileExcluded, nil
}

func GetS3Key(keyPattern string, createdFile event.CreatedFile) string {
	sess := createdFile.Session
	pt := keyPattern
	if sess.Caps.S3KeyPattern != "" {
		pt = sess.Caps.S3KeyPattern
	}
	filename := createdFile.Name
	key := strings.Replace(pt, "$fileName", filepath.Base(filename), -1)
	key = strings.Replace(key, "$fileExtension", strings.ToLower(filepath.Ext(filename)), -1)
	key = strings.Replace(key, "$browserName", strings.ToLower(sess.Caps.BrowserName()), -1)
	key = strings.Replace(key, "$browserVersion", strings.ToLower(sess.Caps.Version), -1)
	key = strings.Replace(key, "$platformName", strings.ToLower(sess.Caps.Platform), -1)
	key = strings.Replace(key, "$quota", strings.ToLower(sess.Quota), -1)
	key = strings.Replace(key, "$sessionId", createdFile.SessionId, -1)
	key = strings.Replace(key, "$fileType", strings.ToLower(createdFile.Type), -1)
	key = strings.Replace(key, "$date", time.Now().Format("2006-01-02"), -1)
	key = strings.Replace(key, " ", "-", -1)
	return key
}
