package agent

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

func TestSaveExecutedSQL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Message{}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	sqlText := "SELECT VendorId FROM Srm_VendorInfo WHERE VendorName = '北京字节跳动科技有限公司'"
	if err := SaveExecutedSQL(db, id, "  "+sqlText+"  "); err != nil {
		t.Fatal(err)
	}
	if err := SaveExecutedSQL(db, uuid.Nil, sqlText); err != nil {
		t.Fatal(err)
	}
	if err := SaveExecutedSQL(nil, id, sqlText); err != nil {
		t.Fatal(err)
	}
	var msgs []model.Message
	if err := db.Where("conversation_id = ?", id).Find(&msgs).Error; err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "tool" || !strings.Contains(msgs[0].Content, sqlText) {
		t.Fatalf("%+v", msgs)
	}
}
