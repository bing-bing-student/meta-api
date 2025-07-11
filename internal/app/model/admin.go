package model

type AdminModel struct {
	*Model
}

// NewAdminModel 创建管理员模型
func NewAdminModel(base *Model) *AdminModel {
	return &AdminModel{Model: base}
}

type Admin struct {
	ID            uint64 `gorm:"primary_key;NOT NULL"`
	Username      string `gorm:"NOT NULL;unique"`
	Password      string `gorm:"NOT NULL"`
	Phone         string `gorm:"NOT NULL;unique"`
	SecretKey     string `gorm:"type:varchar(52)"`
	BindStatus    uint8  `gorm:"NOT NULL;default:0"`
	AboutMeInfo   string `gorm:"type:varchar(1000)"`
	WebSiteInfo   string `gorm:"type:varchar(1000)"`
	ContactMeInfo string `gorm:"type:varchar(500)"`
}

type AdministerInfo struct {
	AboutMeInfo   string `json:"aboutMeInfo" gorm:"about_me_info"`
	WebSiteInfo   string `json:"webSiteInfo" gorm:"web_site_info"`
	ContactMeInfo string `json:"contactMeInfo" gorm:"contact_me_info"`
}

type AboutMeInfo struct {
	Name     string `json:"name"`
	Job      string `json:"job"`
	WorkLife string `json:"workLife"`
	Address  string `json:"address"`
}

type WebSiteInfo struct {
	DomainInfo      string `json:"domainInfo"`
	BlogContent     string `json:"blogContent"`
	WebsiteLocation string `json:"websiteLocation"`
	Statement       string `json:"statement"`
}

type ContactMeInfo struct {
	Email []string `json:"email"`
}

// AddAdminSecretKey 添加管理员密钥
func (a *AdminModel) AddAdminSecretKey(adminID uint64, secretKey string) error {
	if err := a.mysql.Model(&Admin{}).Where("id = ?", adminID).
		Updates(Admin{SecretKey: secretKey, BindStatus: 1}).Error; err != nil {
		return err
	}
	return nil
}

// GetAdminSecretKey 获取管理员密钥
func (a *AdminModel) GetAdminSecretKey(adminID uint64) (string, error) {
	var secretKey string
	if err := a.mysql.Model(&Admin{}).Where("id = ? AND bind_status = ?", adminID, 1).
		Pluck("secret_key", &secretKey).Error; err != nil {
		return "", err
	}
	return secretKey, nil
}
