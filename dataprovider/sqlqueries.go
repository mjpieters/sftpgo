package dataprovider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/drakkan/sftpgo/v2/vfs"
)

const (
	selectUserFields = "id,username,password,public_keys,home_dir,uid,gid,max_sessions,quota_size,quota_files,permissions,used_quota_size," +
		"used_quota_files,last_quota_update,upload_bandwidth,download_bandwidth,expiration_date,last_login,status,filters,filesystem," +
		"additional_info,description,email,created_at,updated_at"
	selectFolderFields = "id,path,used_quota_size,used_quota_files,last_quota_update,name,description,filesystem"
	selectAdminFields  = "id,username,password,status,email,permissions,filters,additional_info,description,created_at,updated_at,last_login"
	selectAPIKeyFields = "key_id,name,api_key,scope,created_at,updated_at,last_use_at,expires_at,description,user_id,admin_id"
)

func getSQLPlaceholders() []string {
	var placeholders []string
	for i := 1; i <= 30; i++ {
		if config.Driver == PGSQLDataProviderName || config.Driver == CockroachDataProviderName {
			placeholders = append(placeholders, fmt.Sprintf("$%v", i))
		} else {
			placeholders = append(placeholders, "?")
		}
	}
	return placeholders
}

func getAdminByUsernameQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v WHERE username = %v`, selectAdminFields, sqlTableAdmins, sqlPlaceholders[0])
}

func getAdminsQuery(order string) string {
	return fmt.Sprintf(`SELECT %v FROM %v ORDER BY username %v LIMIT %v OFFSET %v`, selectAdminFields, sqlTableAdmins,
		order, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getDumpAdminsQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v`, selectAdminFields, sqlTableAdmins)
}

func getAddAdminQuery() string {
	return fmt.Sprintf(`INSERT INTO %v (username,password,status,email,permissions,filters,additional_info,description,created_at,updated_at,last_login)
		VALUES (%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,0)`, sqlTableAdmins, sqlPlaceholders[0], sqlPlaceholders[1],
		sqlPlaceholders[2], sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6], sqlPlaceholders[7],
		sqlPlaceholders[8], sqlPlaceholders[9])
}

func getUpdateAdminQuery() string {
	return fmt.Sprintf(`UPDATE %v SET password=%v,status=%v,email=%v,permissions=%v,filters=%v,additional_info=%v,description=%v,updated_at=%v
		WHERE username = %v`, sqlTableAdmins, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2],
		sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6], sqlPlaceholders[7], sqlPlaceholders[8])
}

func getDeleteAdminQuery() string {
	return fmt.Sprintf(`DELETE FROM %v WHERE username = %v`, sqlTableAdmins, sqlPlaceholders[0])
}

func getAPIKeyByIDQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v WHERE key_id = %v`, selectAPIKeyFields, sqlTableAPIKeys, sqlPlaceholders[0])
}

func getAPIKeysQuery(order string) string {
	return fmt.Sprintf(`SELECT %v FROM %v ORDER BY key_id %v LIMIT %v OFFSET %v`, selectAPIKeyFields, sqlTableAPIKeys,
		order, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getDumpAPIKeysQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v`, selectAPIKeyFields, sqlTableAPIKeys)
}

func getAddAPIKeyQuery() string {
	return fmt.Sprintf(`INSERT INTO %v (key_id,name,api_key,scope,created_at,updated_at,last_use_at,expires_at,description,user_id,admin_id)
		VALUES (%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v)`, sqlTableAPIKeys, sqlPlaceholders[0], sqlPlaceholders[1],
		sqlPlaceholders[2], sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6],
		sqlPlaceholders[7], sqlPlaceholders[8], sqlPlaceholders[9], sqlPlaceholders[10])
}

func getUpdateAPIKeyQuery() string {
	return fmt.Sprintf(`UPDATE %v SET name=%v,scope=%v,expires_at=%v,user_id=%v,admin_id=%v,description=%v,updated_at=%v
		WHERE key_id = %v`, sqlTableAPIKeys, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2],
		sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6], sqlPlaceholders[7])
}

func getDeleteAPIKeyQuery() string {
	return fmt.Sprintf(`DELETE FROM %v WHERE key_id = %v`, sqlTableAPIKeys, sqlPlaceholders[0])
}

func getRelatedUsersForAPIKeysQuery(apiKeys []APIKey) string {
	var sb strings.Builder
	for _, k := range apiKeys {
		if k.userID == 0 {
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString("(")
		} else {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.FormatInt(k.userID, 10))
	}
	if sb.Len() > 0 {
		sb.WriteString(")")
	} else {
		sb.WriteString("(0)")
	}
	return fmt.Sprintf(`SELECT id,username FROM %v WHERE id IN %v`, sqlTableUsers, sb.String())
}

func getRelatedAdminsForAPIKeysQuery(apiKeys []APIKey) string {
	var sb strings.Builder
	for _, k := range apiKeys {
		if k.adminID == 0 {
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString("(")
		} else {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.FormatInt(k.adminID, 10))
	}
	if sb.Len() > 0 {
		sb.WriteString(")")
	} else {
		sb.WriteString("(0)")
	}
	return fmt.Sprintf(`SELECT id,username FROM %v WHERE id IN %v`, sqlTableAdmins, sb.String())
}

func getUserByUsernameQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v WHERE username = %v`, selectUserFields, sqlTableUsers, sqlPlaceholders[0])
}

func getUsersQuery(order string) string {
	return fmt.Sprintf(`SELECT %v FROM %v ORDER BY username %v LIMIT %v OFFSET %v`, selectUserFields, sqlTableUsers,
		order, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getRecentlyUpdatedUsersQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v WHERE updated_at >= %v`, selectUserFields, sqlTableUsers, sqlPlaceholders[0])
}

func getDumpUsersQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v`, selectUserFields, sqlTableUsers)
}

func getDumpFoldersQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v`, selectFolderFields, sqlTableFolders)
}

func getUpdateQuotaQuery(reset bool) string {
	if reset {
		return fmt.Sprintf(`UPDATE %v SET used_quota_size = %v,used_quota_files = %v,last_quota_update = %v
			WHERE username = %v`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3])
	}
	return fmt.Sprintf(`UPDATE %v SET used_quota_size = used_quota_size + %v,used_quota_files = used_quota_files + %v,last_quota_update = %v
		WHERE username = %v`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3])
}

func getSetUpdateAtQuery() string {
	return fmt.Sprintf(`UPDATE %v SET updated_at = %v WHERE username = %v`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getUpdateLastLoginQuery() string {
	return fmt.Sprintf(`UPDATE %v SET last_login = %v WHERE username = %v`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getUpdateAdminLastLoginQuery() string {
	return fmt.Sprintf(`UPDATE %v SET last_login = %v WHERE username = %v`, sqlTableAdmins, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getUpdateAPIKeyLastUseQuery() string {
	return fmt.Sprintf(`UPDATE %v SET last_use_at = %v WHERE key_id = %v`, sqlTableAPIKeys, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getQuotaQuery() string {
	return fmt.Sprintf(`SELECT used_quota_size,used_quota_files FROM %v WHERE username = %v`, sqlTableUsers,
		sqlPlaceholders[0])
}

func getAddUserQuery() string {
	return fmt.Sprintf(`INSERT INTO %v (username,password,public_keys,home_dir,uid,gid,max_sessions,quota_size,quota_files,permissions,
		used_quota_size,used_quota_files,last_quota_update,upload_bandwidth,download_bandwidth,status,last_login,expiration_date,filters,
		filesystem,additional_info,description,email,created_at,updated_at)
		VALUES (%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,0,0,0,%v,%v,%v,0,%v,%v,%v,%v,%v,%v,%v,%v)`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1],
		sqlPlaceholders[2], sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6], sqlPlaceholders[7],
		sqlPlaceholders[8], sqlPlaceholders[9], sqlPlaceholders[10], sqlPlaceholders[11], sqlPlaceholders[12], sqlPlaceholders[13],
		sqlPlaceholders[14], sqlPlaceholders[15], sqlPlaceholders[16], sqlPlaceholders[17], sqlPlaceholders[18], sqlPlaceholders[19],
		sqlPlaceholders[20])
}

func getUpdateUserQuery() string {
	return fmt.Sprintf(`UPDATE %v SET password=%v,public_keys=%v,home_dir=%v,uid=%v,gid=%v,max_sessions=%v,quota_size=%v,
		quota_files=%v,permissions=%v,upload_bandwidth=%v,download_bandwidth=%v,status=%v,expiration_date=%v,filters=%v,filesystem=%v,
		additional_info=%v,description=%v,email=%v,updated_at=%v WHERE id = %v`, sqlTableUsers, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3],
		sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6], sqlPlaceholders[7], sqlPlaceholders[8], sqlPlaceholders[9],
		sqlPlaceholders[10], sqlPlaceholders[11], sqlPlaceholders[12], sqlPlaceholders[13], sqlPlaceholders[14], sqlPlaceholders[15],
		sqlPlaceholders[16], sqlPlaceholders[17], sqlPlaceholders[18], sqlPlaceholders[19])
}

func getDeleteUserQuery() string {
	return fmt.Sprintf(`DELETE FROM %v WHERE id = %v`, sqlTableUsers, sqlPlaceholders[0])
}

func getFolderByNameQuery() string {
	return fmt.Sprintf(`SELECT %v FROM %v WHERE name = %v`, selectFolderFields, sqlTableFolders, sqlPlaceholders[0])
}

func checkFolderNameQuery() string {
	return fmt.Sprintf(`SELECT name FROM %v WHERE name = %v`, sqlTableFolders, sqlPlaceholders[0])
}

func getAddFolderQuery() string {
	return fmt.Sprintf(`INSERT INTO %v (path,used_quota_size,used_quota_files,last_quota_update,name,description,filesystem)
		VALUES (%v,%v,%v,%v,%v,%v,%v)`, sqlTableFolders, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2],
		sqlPlaceholders[3], sqlPlaceholders[4], sqlPlaceholders[5], sqlPlaceholders[6])
}

func getUpdateFolderQuery() string {
	return fmt.Sprintf(`UPDATE %v SET path=%v,description=%v,filesystem=%v WHERE name = %v`, sqlTableFolders, sqlPlaceholders[0],
		sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3])
}

func getDeleteFolderQuery() string {
	return fmt.Sprintf(`DELETE FROM %v WHERE id = %v`, sqlTableFolders, sqlPlaceholders[0])
}

func getClearFolderMappingQuery() string {
	return fmt.Sprintf(`DELETE FROM %v WHERE user_id = (SELECT id FROM %v WHERE username = %v)`, sqlTableFoldersMapping,
		sqlTableUsers, sqlPlaceholders[0])
}

func getAddFolderMappingQuery() string {
	return fmt.Sprintf(`INSERT INTO %v (virtual_path,quota_size,quota_files,folder_id,user_id)
		VALUES (%v,%v,%v,%v,(SELECT id FROM %v WHERE username = %v))`, sqlTableFoldersMapping, sqlPlaceholders[0],
		sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3], sqlTableUsers, sqlPlaceholders[4])
}

func getFoldersQuery(order string) string {
	return fmt.Sprintf(`SELECT %v FROM %v ORDER BY name %v LIMIT %v OFFSET %v`, selectFolderFields, sqlTableFolders,
		order, sqlPlaceholders[0], sqlPlaceholders[1])
}

func getUpdateFolderQuotaQuery(reset bool) string {
	if reset {
		return fmt.Sprintf(`UPDATE %v SET used_quota_size = %v,used_quota_files = %v,last_quota_update = %v
			WHERE name = %v`, sqlTableFolders, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3])
	}
	return fmt.Sprintf(`UPDATE %v SET used_quota_size = used_quota_size + %v,used_quota_files = used_quota_files + %v,last_quota_update = %v
		WHERE name = %v`, sqlTableFolders, sqlPlaceholders[0], sqlPlaceholders[1], sqlPlaceholders[2], sqlPlaceholders[3])
}

func getQuotaFolderQuery() string {
	return fmt.Sprintf(`SELECT used_quota_size,used_quota_files FROM %v WHERE name = %v`, sqlTableFolders,
		sqlPlaceholders[0])
}

func getRelatedFoldersForUsersQuery(users []User) string {
	var sb strings.Builder
	for _, u := range users {
		if sb.Len() == 0 {
			sb.WriteString("(")
		} else {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.FormatInt(u.ID, 10))
	}
	if sb.Len() > 0 {
		sb.WriteString(")")
	}
	return fmt.Sprintf(`SELECT f.id,f.name,f.path,f.used_quota_size,f.used_quota_files,f.last_quota_update,fm.virtual_path,
		fm.quota_size,fm.quota_files,fm.user_id,f.filesystem,f.description FROM %v f INNER JOIN %v fm ON f.id = fm.folder_id WHERE
		fm.user_id IN %v ORDER BY fm.user_id`, sqlTableFolders, sqlTableFoldersMapping, sb.String())
}

func getRelatedUsersForFoldersQuery(folders []vfs.BaseVirtualFolder) string {
	var sb strings.Builder
	for _, f := range folders {
		if sb.Len() == 0 {
			sb.WriteString("(")
		} else {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.FormatInt(f.ID, 10))
	}
	if sb.Len() > 0 {
		sb.WriteString(")")
	}
	return fmt.Sprintf(`SELECT fm.folder_id,u.username FROM %v fm INNER JOIN %v u ON fm.user_id = u.id
		WHERE fm.folder_id IN %v ORDER BY fm.folder_id`, sqlTableFoldersMapping, sqlTableUsers, sb.String())
}

func getDatabaseVersionQuery() string {
	return fmt.Sprintf("SELECT version from %v LIMIT 1", sqlTableSchemaVersion)
}

func getUpdateDBVersionQuery() string {
	return fmt.Sprintf(`UPDATE %v SET version=%v`, sqlTableSchemaVersion, sqlPlaceholders[0])
}
