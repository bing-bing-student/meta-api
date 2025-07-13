package tag

type Tag struct {
	ID   uint64 `gorm:"primary_key;NOT NULL"`
	Name string `gorm:"NOT NULL;unique;"`
}
