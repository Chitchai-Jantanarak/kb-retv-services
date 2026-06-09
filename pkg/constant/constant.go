package constant

type (
	StoreRentalStatusEnum         string
	StoreRentalLoginAlgorithmEnum string
	StoreRentalLoginPluginEnum    string
	StoreRentalDatabaseTypeEnum   string
	StoreRentalServerTypeEnum     string
)

const (
	StoreRentalStatusActive   StoreRentalStatusEnum = "ACTIVE"
	StoreRentalStatusInactive StoreRentalStatusEnum = "INACTIVE"
	StoreRentalStatusBan      StoreRentalStatusEnum = "BAN"
	StoreRentalStatusExpired  StoreRentalStatusEnum = "EXPIRED"

	StoreRentalDatabaseTypeMySQL   StoreRentalDatabaseTypeEnum = "MYSQL"
	StoreRentalDatabaseTypePostgre StoreRentalDatabaseTypeEnum = "POSTGRESQL"
	StoreRentalDatabaseTypeNoting  StoreRentalDatabaseTypeEnum = "NOTING"

	StoreRentalServerTypeOnlineMode  StoreRentalServerTypeEnum = "ONLINEMODE"
	StoreRentalServerTypeOfflineMode StoreRentalServerTypeEnum = "OFFLINEMODE"

	StoreRentalLoginPluginEnumAuthme     StoreRentalLoginPluginEnum = "AUTHME"
	StoreRentalLoginPluginEnumLibrelogin StoreRentalLoginPluginEnum = "LIBRELOGIN"
	StoreRentalLoginPluginEnumJpremium   StoreRentalLoginPluginEnum = "JPREMIUM"
	StoreRentalLoginPluginEnumOther      StoreRentalLoginPluginEnum = "OTHER"
	StoreRentalLoginPluginEnumNothing    StoreRentalLoginPluginEnum = "NOTHING"

	StoreRentalLoginAlgorithmEnumSHA256    StoreRentalLoginAlgorithmEnum = "SHA256"
	StoreRentalLoginAlgorithmEnumSHA_256   StoreRentalLoginAlgorithmEnum = "SHA-256"
	StoreRentalLoginAlgorithmEnumSHA512    StoreRentalLoginAlgorithmEnum = "SHA512"
	StoreRentalLoginAlgorithmEnumSHA_512   StoreRentalLoginAlgorithmEnum = "SHA-512"
	StoreRentalLoginAlgorithmEnumBCRYPT    StoreRentalLoginAlgorithmEnum = "BCRYPT"
	StoreRentalLoginAlgorithmEnumBCRYPT_2A StoreRentalLoginAlgorithmEnum = "BCRYPT-2A"
	StoreRentalLoginAlgorithmEnumBCRYPT2Y  StoreRentalLoginAlgorithmEnum = "BCRYPT2Y"
	StoreRentalLoginAlgorithmEnumARGON2    StoreRentalLoginAlgorithmEnum = "ARGON2"
	StoreRentalLoginAlgorithmEnumARGON_2ID StoreRentalLoginAlgorithmEnum = "ARGON-2ID"
	StoreRentalLoginAlgorithmEnumSHA256j   StoreRentalLoginAlgorithmEnum = "SHA256j"
)
