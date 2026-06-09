package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/my/app/pkg/constant"
)

type HashedPassword struct {
	Hash string
	Salt string
}

type BiHolder struct {
	Key   string
	Value string
}
type PasswordProps struct {
	Password     string
	PasswordHash *string
	Salt         *string
	Plugin       *constant.StoreRentalLoginPluginEnum
}

// Bcrypt functions
func BcryptConvertHash(hash string) (BiHolder, error) {
	parts := strings.Split(hash, "$")
	if len(parts) < 2 {
		return BiHolder{}, errors.New("invalid hash")
	}
	return BiHolder{Key: parts[0], Value: parts[1]}, nil
}

func RawBcryptFromHashed(password HashedPassword) (string, error) {
	bi, err := BcryptConvertHash(password.Hash)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$2a$%s$%s%s", bi.Key, password.Salt, bi.Value), nil
}

func ConvertFromBCryptRaw(rawHash string) (*HashedPassword, error) {
	parts := strings.Split(rawHash, "$")
	if len(parts) != 4 {
		return nil, errors.New("invalid hash format: expected 4 parts")
	}

	cost := parts[2]
	saltAndHash := parts[3]
	if len(saltAndHash) < 22 {
		return nil, errors.New("invalid salt/hash length: expected at least 22 characters")
	}

	salt := saltAndHash[:22]
	hash := saltAndHash[22:]

	return &HashedPassword{
		Hash: fmt.Sprintf("%s$%s", cost, hash),
		Salt: salt,
	}, nil
}

func BcryptCreateHash(password string) (*HashedPassword, error) {
	rawHash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate bcrypt hash: %w", err)
	}

	hashedPassword, err := ConvertFromBCryptRaw(string(rawHash))
	if err != nil {
		return nil, fmt.Errorf("failed to convert bcrypt hash: %w", err)
	}

	return hashedPassword, nil
}

func BcryptVerify(input string, hash HashedPassword) (bool, error) {
	raw, err := RawBcryptFromHashed(hash)
	if err != nil {
		return false, fmt.Errorf("failed to convert hashed password: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(raw), []byte(input))
	return err == nil, nil
}

//---------------------------------------------------------------

// Argon2id functions
func getSalt() ([]byte, error) {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random salt: %w", err)
	}
	return salt, nil
}

func convertFromArgon2ID(hash []byte, salt []byte) HashedPassword {
	hashBase64 := base64.StdEncoding.EncodeToString(hash)
	saltBase64 := base64.StdEncoding.EncodeToString(salt)
	formattedHash := fmt.Sprintf("19,3,16384$%s", hashBase64)
	return HashedPassword{Hash: formattedHash, Salt: saltBase64}
}

func argon2Hash(password string) (HashedPassword, error) {
	salt, err := getSalt()
	if err != nil {
		return HashedPassword{}, err
	}
	timeCost := uint32(3)
	memoryCost := uint32(16384)
	parallelism := uint8(1)
	keyLen := uint32(32)

	hash := argon2.IDKey([]byte(password), salt, timeCost, memoryCost, parallelism, keyLen)

	return convertFromArgon2ID(hash, salt), nil
}

func argon2Verify(input string, hash HashedPassword) (bool, error) {
	parts := strings.Split(hash.Hash, "$")
	if len(parts) != 2 {
		return false, errors.New("invalid hash format: expected 2 parts")
	}

	params := parts[0]
	hashBase64 := parts[1]

	var version, timeCost, memoryCost uint32
	_, err := fmt.Sscanf(params, "%d,%d,%d", &version, &timeCost, &memoryCost)
	if err != nil {
		return false, fmt.Errorf("invalid parameters format: %w", err)
	}

	if version != 19 {
		return false, errors.New("unsupported argon2id version")
	}

	salt, err := base64.StdEncoding.DecodeString(hash.Salt)
	if err != nil {
		return false, fmt.Errorf("invalid salt encoding: %w", err)
	}

	expectedHash, err := base64.StdEncoding.DecodeString(hashBase64)
	if err != nil {
		return false, fmt.Errorf("invalid hash encoding: %w", err)
	}

	parallelism := uint8(1)
	inputHash := argon2.IDKey([]byte(input), salt, timeCost, memoryCost, parallelism, uint32(len(expectedHash)))

	return subtle.ConstantTimeCompare(inputHash, expectedHash) == 1, nil
}

// func argon2HashSalt(hash string) (HashedPassword, error) {
// 	parts := strings.Split(hash, "$")
// 	if len(parts) != 3 {
// 		return HashedPassword{}, errors.New("invalid hash format: expected 3 parts")
// 	}

// 	params := parts[0]
// 	salt := parts[1]
// 	hashBase64 := parts[2]

// 	var version, timeCost, memoryCost uint32
// 	_, err := fmt.Sscanf(params, "%d,%d,%d", &version, &timeCost, &memoryCost)
// 	if err != nil {
// 		return HashedPassword{}, fmt.Errorf("invalid parameters format: %w", err)
// 	}

// 	formattedHash := fmt.Sprintf("%s$%s", params, hashBase64)

// 	return HashedPassword{Hash: formattedHash, Salt: salt}, nil
// }

//---------------------------------------------------------------

// SHA functions
func doubleHashSHA256(pw, salt string) string {
	first := sha256.Sum256([]byte(pw))
	second := sha256.Sum256([]byte(hex.EncodeToString(first[:]) + salt))
	return hex.EncodeToString(second[:])
}

func doubleHashSHA512(pw, salt string) string {
	first := sha512.Sum512([]byte(pw))
	second := sha512.Sum512([]byte(hex.EncodeToString(first[:]) + salt))
	return hex.EncodeToString(second[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------
func PasswordHash(data PasswordProps, algo constant.StoreRentalLoginAlgorithmEnum) (*string, *string, error) {
	salt := ""
	if data.Plugin != nil && *data.Plugin == constant.StoreRentalLoginPluginEnumLibrelogin || isSHAorArgon(algo) {
		salt = randomHex(16)
	}

	switch algo {
	case constant.StoreRentalLoginAlgorithmEnumSHA_256, constant.StoreRentalLoginAlgorithmEnumSHA256, constant.StoreRentalLoginAlgorithmEnumSHA256j:
		hash := fmt.Sprintf("$SHA$%s$%s", salt, doubleHashSHA256(data.Password, salt))
		return &hash, &salt, nil
	case constant.StoreRentalLoginAlgorithmEnumSHA_512:
		hash := fmt.Sprintf("$SHA$%s$%s", salt, doubleHashSHA512(data.Password, salt))
		return &hash, &salt, nil
	case constant.StoreRentalLoginAlgorithmEnumBCRYPT, constant.StoreRentalLoginAlgorithmEnumBCRYPT2Y, constant.StoreRentalLoginAlgorithmEnumBCRYPT_2A:
		h, err := BcryptCreateHash(data.Password)
		if err != nil {
			return nil, nil, err
		}
		return &h.Hash, &h.Salt, nil
	case constant.StoreRentalLoginAlgorithmEnumARGON2, constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
		h, err := argon2Hash(data.Password)
		if err != nil {
			return nil, nil, err
		}
		return &h.Hash, &h.Salt, nil
	default:
		return nil, nil, errors.New("unsupported algorithm")
	}
}

func ConvertToHashedStruct(hashStr string, algo constant.StoreRentalLoginAlgorithmEnum) *HashedPassword {
	switch algo {
	case constant.StoreRentalLoginAlgorithmEnumBCRYPT, constant.StoreRentalLoginAlgorithmEnumBCRYPT2Y, constant.StoreRentalLoginAlgorithmEnumBCRYPT_2A:
		hashedPassword, err := ConvertFromBCryptRaw(hashStr)
		if err != nil {
			return nil
		}
		return hashedPassword
	case constant.StoreRentalLoginAlgorithmEnumARGON2, constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
		parts := strings.Split(hashStr, "$")
		if len(parts) != 2 {
			return nil
		}
		return &HashedPassword{Hash: hashStr, Salt: parts[4]}
	case constant.StoreRentalLoginAlgorithmEnumSHA_256, constant.StoreRentalLoginAlgorithmEnumSHA256, constant.StoreRentalLoginAlgorithmEnumSHA256j, constant.StoreRentalLoginAlgorithmEnumSHA_512:
		parts := strings.Split(hashStr, "$")
		if len(parts) < 3 {
			return nil
		}
		return &HashedPassword{Hash: hashStr, Salt: parts[2]}
	}
	return nil
}

func isSHAorArgon(algo constant.StoreRentalLoginAlgorithmEnum) bool {
	switch algo {
	case constant.StoreRentalLoginAlgorithmEnumSHA_256,
		constant.StoreRentalLoginAlgorithmEnumSHA256,
		constant.StoreRentalLoginAlgorithmEnumSHA256j,
		constant.StoreRentalLoginAlgorithmEnumSHA_512,
		constant.StoreRentalLoginAlgorithmEnumARGON2,
		constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
		return true
	default:
		return false
	}
}

func PasswordVerify(data PasswordProps, algo constant.StoreRentalLoginAlgorithmEnum) (bool, error) {
	switch algo {
	case constant.StoreRentalLoginAlgorithmEnumSHA_256, constant.StoreRentalLoginAlgorithmEnumSHA256, constant.StoreRentalLoginAlgorithmEnumSHA256j:
		if data.Salt == nil {
			parts := strings.Split(*data.PasswordHash, "$")
			if len(parts) < 3 {
				return false, nil
			}
			data.Salt = &parts[2]
		}
		hash := fmt.Sprintf("$SHA$%s$%s", *data.Salt, doubleHashSHA256(data.Password, *data.Salt))
		return hash == *data.PasswordHash, nil
	case constant.StoreRentalLoginAlgorithmEnumSHA_512:
		if data.Salt == nil {
			parts := strings.Split(*data.PasswordHash, "$")
			if len(parts) < 3 {
				return false, nil
			}
			data.Salt = &parts[2]
		}
		hash := fmt.Sprintf("$SHA$%s$%s", *data.Salt, doubleHashSHA512(data.Password, *data.Salt))
		return hash == *data.PasswordHash, nil
	case constant.StoreRentalLoginAlgorithmEnumBCRYPT, constant.StoreRentalLoginAlgorithmEnumBCRYPT2Y, constant.StoreRentalLoginAlgorithmEnumBCRYPT_2A:
		return BcryptVerify(data.Password, HashedPassword{Hash: *data.PasswordHash, Salt: *data.Salt})
	case constant.StoreRentalLoginAlgorithmEnumARGON2, constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
		return argon2Verify(data.Password, HashedPassword{Hash: *data.PasswordHash, Salt: *data.Salt})
	default:
		return false, errors.New("unsupported algorithm")
	}
}

// func PasswordHash(data PasswordProps, algo constant.StoreRentalLoginAlgorithmEnum) (*string, *string, error) {
// 	if data.Plugin != nil && *data.Plugin == constant.StoreRentalLoginPluginEnumLibrelogin {
// 		switch algo {
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_256:
// 			salt := randomHex(16)
// 			hash := "$SHA$" + salt + "$" + doubleHashSHA256(data.Password, salt)
// 			return &hash, &salt, nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_512:
// 			salt := randomHex(16)
// 			hash := "$SHA$" + salt + "$" + doubleHashSHA512(data.Password, salt)
// 			return &hash, &salt, nil
// 		case constant.StoreRentalLoginAlgorithmEnumBCRYPT_2A:
// 			hash, err := bcryptCreateHash(data.Password)
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 			return &hash.Hash, &hash.Salt, nil
// 		case constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
// 			hash, err := argon2Hash(data.Password)
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 			return &hash.Hash, &hash.Salt, nil
// 		}
// 	} else {
// 		switch algo {
// 		case constant.StoreRentalLoginAlgorithmEnumSHA256:
// 			salt := randomHex(16)
// 			hash := "$SHA$" + salt + "$" + doubleHashSHA256(data.Password, salt)
// 			return &hash, nil, nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_512:
// 			salt := randomHex(16)
// 			hash := "SHA512$" + salt + "$" + doubleHashSHA512(data.Password, salt)
// 			return &hash, nil, nil
// 		case constant.StoreRentalLoginAlgorithmEnumBCRYPT, constant.StoreRentalLoginAlgorithmEnumBCRYPT2Y:
// 			hash, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
// 			if err != nil {
// 				return nil, nil, err
// 			}
// 			hashStr := string(hash)
// 			return &hashStr, nil, nil
// 		case constant.StoreRentalLoginAlgorithmEnumARGON2:
// 			salt := randomHex(16)
// 			hash := argon2.IDKey([]byte(data.Password), []byte(salt), 3, 16384, 1, 32)
// 			hashStr := string(hash)
// 			return &hashStr, nil, nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA256j:
// 			salt := randomHex(16)
// 			hash := "SHA256$" + salt + "$" + doubleHashSHA256(data.Password, salt)
// 			return &hash, nil, nil
// 		}
// 	}
// 	return nil, nil, errors.New("unsupported algorithm")
// }

// func PasswordVerify(data PasswordProps, algo constant.StoreRentalLoginAlgorithmEnum) (bool, error) {
// 	if data.Plugin != nil && *data.Plugin == constant.StoreRentalLoginPluginEnumLibrelogin {
// 		switch algo {
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_256:
// 			hash := "$SHA$" + *data.Salt + "$" + doubleHashSHA256(data.Password, *data.Salt)
// 			return hash == *data.PasswordHash, nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_512:
// 			hash := "$SHA$" + *data.Salt + "$" + doubleHashSHA512(data.Password, *data.Salt)
// 			return hash == *data.PasswordHash, nil
// 		case constant.StoreRentalLoginAlgorithmEnumBCRYPT_2A:
// 			if len(*data.PasswordHash) < 60 {
// 				return false, nil
// 			}
// 			bcrypth, err := bcryptVerify(data.Password, HashedPassword{Hash: *data.PasswordHash, Salt: *data.Salt})
// 			if err != nil {
// 				return false, err
// 			}
// 			return bcrypth, nil
// 		case constant.StoreRentalLoginAlgorithmEnumARGON_2ID:
// 			expected, err := argon2Verify(data.Password, HashedPassword{Hash: *data.PasswordHash, Salt: *data.Salt})
// 			if err != nil {
// 				return false, err
// 			}
// 			return expected, nil
// 		}
// 	} else {
// 		switch algo {
// 		case constant.StoreRentalLoginAlgorithmEnumSHA256:
// 			parts := strings.Split(*data.PasswordHash, "$")
// 			if len(parts) < 3 {
// 				return false, nil
// 			}
// 			salt := parts[2]
// 			hash := "$SHA$" + salt + "$" + doubleHashSHA256(data.Password, salt)
// 			return hash == *data.PasswordHash, nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA_512:
// 			parts := strings.Split(*data.PasswordHash, "$")
// 			if len(parts) < 3 {
// 				return false, nil
// 			}
// 			salt := parts[1]
// 			expected := doubleHashSHA512(data.Password, salt)
// 			return expected == parts[2], nil
// 		case constant.StoreRentalLoginAlgorithmEnumSHA256j:
// 			parts := strings.Split(*data.PasswordHash, "$")
// 			if len(parts) < 4 {
// 				return false, nil
// 			}
// 			salt := parts[2]
// 			expected := doubleHashSHA256(data.Password, salt)
// 			return expected == parts[3], nil
// 		case constant.StoreRentalLoginAlgorithmEnumBCRYPT:
// 			if len(*data.PasswordHash) < 60 {
// 				return false, nil
// 			}
// 			return bcrypt.CompareHashAndPassword([]byte(*data.PasswordHash), []byte(data.Password)) == nil, nil
// 		case constant.StoreRentalLoginAlgorithmEnumARGON2:
// 			ext, err := argon2HashSalt(*data.PasswordHash)
// 			if err != nil {
// 				return false, err
// 			}
// 			argonCehck, err := argon2Verify(data.Password, HashedPassword{Hash: ext.Hash, Salt: ext.Salt})
// 			if err != nil {
// 				return false, err
// 			}
// 			return argonCehck, nil

// 		}
// 	}
// 	return false, nil
// }
