package config

import "github.com/conductorone/baton-sdk/pkg/field"

var (
	TeleportKeyFileField = field.StringField(
		"teleport-key-file",
		field.WithRequired(true),
		field.WithDescription("Path to the teleport file generated by using the tctl admin tool. Example: \"auth.pem\"."),
	)
	ProxyAddressField = field.StringField(
		"teleport-proxy-address",
		field.WithRequired(true),
		field.WithDescription("The fully-qualified teleport proxy service to connect with. Example: \"baton.teleport.sh:443\"."),
	)
	ConfigurationFields = []field.SchemaField{ProxyAddressField, TeleportKeyFileField}
	ConfigurationSchema = field.NewConfiguration(ConfigurationFields)
)