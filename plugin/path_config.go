//
// Copyright 2021 Outfox, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package jwtsecrets

import (
	"context"
	"gopkg.in/square/go-jose.v2"
	"regexp"
	"time"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

const (
	keySignatureAlgorithm  = "sig_alg"
	keyRSAKeyBits          = "rsa_key_bits"
	keyRotationDuration    = "key_ttl"
	keyTokenTTL            = "jwt_ttl"
	keySetIAT              = "set_iat"
	keySetJTI              = "set_jti"
	keySetNBF              = "set_nbf"
	keyAudiencePattern     = "audience_pattern"
	keySubjectPattern      = "subject_pattern"
	keyMaxAllowedAudiences = "max_audiences"
	keyAllowedClaims       = "allowed_claims"
)

func pathConfig(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "config",
		Fields: map[string]*framework.FieldSchema{
			keySignatureAlgorithm: {
				Type:        framework.TypeString,
				Description: `Signature algorithm used to sign new tokens.`,
			},
			keyRSAKeyBits: {
				Type:        framework.TypeInt,
				Description: `Size of generated RSA keys, when signature algorithm is one of the allowed RSA signing algorithm.`,
			},
			keyRotationDuration: {
				Type:        framework.TypeString,
				Description: `Duration a specific key will be used to sign new tokens.`,
			},
			keyTokenTTL: {
				Type:        framework.TypeString,
				Description: `Duration a token is valid for (mapped to the 'exp' claim).`,
			},
			keySetIAT: {
				Type:        framework.TypeBool,
				Description: `Whether or not the backend should generate and set the 'iat' claim.`,
			},
			keySetJTI: {
				Type:        framework.TypeBool,
				Description: `Whether or not the backend should generate and set the 'jti' claim.`,
			},
			keySetNBF: {
				Type:        framework.TypeBool,
				Description: `Whether or not the backend should generate and set the 'nbf' claim.`,
			},
			keyIssuer: {
				Type:        framework.TypeString,
				Description: `Value to set as the 'iss' claim. Claim is omitted if empty.`,
			},
			keyAudiencePattern: {
				Type:        framework.TypeString,
				Description: `Regular expression which must match incoming 'aud' claims.`,
			},
			keySubjectPattern: {
				Type:        framework.TypeString,
				Description: `Regular expression which must match incoming 'sub' claims`,
			},
			keyMaxAllowedAudiences: {
				Type:        framework.TypeInt,
				Description: `Maximum number of allowed audiences, or -1 for no limit.`,
			},
			keyAllowedClaims: {
				Type: framework.TypeStringSlice,
				Description: `Claims which are able to be set in addition to ones generated by the backend.
Note: 'aud' and 'sub' should be in this list if you would like to set them.`,
			},
		},

		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.pathConfigRead,
			},
			logical.CreateOperation: &framework.PathOperation{
				Callback: b.pathConfigWrite,
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathConfigWrite,
			},
			logical.DeleteOperation: &framework.PathOperation{
				Callback: b.pathConfigDelete,
			},
		},

		ExistenceCheck:  b.pathConfigExistenceCheck,
		HelpSynopsis:    pathConfigHelpSyn,
		HelpDescription: pathConfigHelpDesc,
	}
}

func (b *backend) pathConfigExistenceCheck(ctx context.Context, req *logical.Request, _ *framework.FieldData) (bool, error) {
	savedConfig, err := req.Storage.Get(ctx, configPath)
	if err != nil {
		return false, err
	}

	return savedConfig != nil, nil
}

func (b *backend) pathConfigWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	config, err := b.getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	if newRawSignatureAlgorithmName, ok := d.GetOk(keySignatureAlgorithm); ok {
		newSignatureAlgorithmName, ok := newRawSignatureAlgorithmName.(string)
		if !ok {
			return logical.ErrorResponse("sig_alg must be a string"), logical.ErrInvalidRequest
		}
		if !stringInSlice(newSignatureAlgorithmName, AllowedSignatureAlgorithmNames) {
			return logical.ErrorResponse("unknown/unsupported signature algorithm, must be one of %s", AllowedSignatureAlgorithmNames), logical.ErrInvalidRequest
		}
		config.SignatureAlgorithm = jose.SignatureAlgorithm(newSignatureAlgorithmName)
	}

	if newRawRSAKeyBits, ok := d.GetOk(keyRSAKeyBits); ok {
		newRSAKeyBits, ok := newRawRSAKeyBits.(int)
		if !ok {
			return logical.ErrorResponse("rsa_key_bits must be an integer"), logical.ErrInvalidRequest
		}
		if !intInSlice(newRSAKeyBits, AllowedRSAKeyBits) {
			return logical.ErrorResponse("unsupported rsa_key_bits, must be one of %s", AllowedRSAKeyBits), logical.ErrInvalidRequest
		}
		config.RSAKeyBits = newRSAKeyBits
	}

	if newRotationPeriod, ok := d.GetOk(keyRotationDuration); ok {
		duration, err := time.ParseDuration(newRotationPeriod.(string))
		if err != nil {
			return nil, err
		}
		config.KeyRotationPeriod = duration
	}

	if newTTL, ok := d.GetOk(keyTokenTTL); ok {
		duration, err := time.ParseDuration(newTTL.(string))
		if err != nil {
			return nil, err
		}
		config.TokenTTL = duration
	}

	if newSetIat, ok := d.GetOk(keySetIAT); ok {
		config.SetIAT = newSetIat.(bool)
	}

	if newSetJTI, ok := d.GetOk(keySetJTI); ok {
		config.SetJTI = newSetJTI.(bool)
	}

	if newSetNBF, ok := d.GetOk(keySetNBF); ok {
		config.SetNBF = newSetNBF.(bool)
	}

	if newAudiencePattern, ok := d.GetOk(keyAudiencePattern); ok {
		config.AudiencePattern = newAudiencePattern.(string)
		_, err := regexp.Compile(config.AudiencePattern)
		if err != nil {
			return logical.ErrorResponse("invalid audience pattern"), err
		}
	}

	if newSubjectPattern, ok := d.GetOk(keySubjectPattern); ok {
		config.SubjectPattern = newSubjectPattern.(string)
		_, err := regexp.Compile(config.SubjectPattern)
		if err != nil {
			return logical.ErrorResponse("invalid subject pattern"), err
		}
	}

	if newMaxAudiences, ok := d.GetOk(keyMaxAllowedAudiences); ok {
		config.MaxAudiences = newMaxAudiences.(int)
	}

	if newAllowedClaims, ok := d.GetOk(keyAllowedClaims); ok {

		// Check allowed claims doesn't contain reserved claims
		for _, newAllowedClaim := range newAllowedClaims.([]string) {
			if stringInSlice(newAllowedClaim, ReservedClaims) {
				return logical.ErrorResponse("'%s' claim is reserved and not permitted in allowed_claims", newAllowedClaim), logical.ErrInvalidRequest
			}
		}

		config.AllowedClaims = newAllowedClaims.([]string)
	}

	if config.TokenTTL > b.System().MaxLeaseTTL() {
		return logical.ErrorResponse("'%s' is greater that the max lease ttl", keyTokenTTL), logical.ErrInvalidRequest
	}

	if err := b.saveConfig(ctx, req.Storage, config); err != nil {
		return nil, err
	}

	return configResponse(config)
}

func (b *backend) pathConfigRead(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	config, err := b.getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	return configResponse(config)
}

func (b *backend) pathConfigDelete(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	err := b.clearConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func configResponse(config *Config) (*logical.Response, error) {
	return &logical.Response{
		Data: map[string]interface{}{
			keySignatureAlgorithm:  config.SignatureAlgorithm,
			keyRSAKeyBits:          config.RSAKeyBits,
			keyRotationDuration:    config.KeyRotationPeriod.String(),
			keyTokenTTL:            config.TokenTTL.String(),
			keySetIAT:              config.SetIAT,
			keySetJTI:              config.SetJTI,
			keySetNBF:              config.SetNBF,
			keyAudiencePattern:     config.AudiencePattern,
			keySubjectPattern:      config.SubjectPattern,
			keyMaxAllowedAudiences: config.MaxAudiences,
			keyAllowedClaims:       config.AllowedClaims,
		},
	}, nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func intInSlice(a int, list []int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

const pathConfigHelpSyn = `
Configure the backend.
`

const pathConfigHelpDesc = `
Configure the backend.

sig_alg:		  Signature algorithm used to sign new tokens.
rsa_key_bits:	  Size of generate RSA keys, when using RSA signature algorithms.
key_ttl:          Duration before a key stops signing new tokens and a new one is generated.
		          After this period the public key will still be available to verify JWTs.
jwt_ttl:          Duration before a token expires.
set_iat:          Whether or not the backend should generate and set the 'iat' claim.
set_jti:          Whether or not the backend should generate and set the 'jti' claim.
set_nbf:          Whether or not the backend should generate and set the 'nbf' claim.
issuer:           Value to set as the 'iss' claim. Claim omitted if empty.
audience_pattern: Regular expression which must match incoming 'aud' claims.
subject_pattern:  Regular expression which must match incoming 'sub' claims.
max_audiences:    Maximum number of allowed audiences, or -1 for no limit.
allowed_claims:   Claims which are able to be set in addition to ones generated by the backend.
                  Note: 'aud' and 'sub' should be in this list if you would like to set them.
`
