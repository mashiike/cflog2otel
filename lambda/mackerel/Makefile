export AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query 'Account' --output text)
.PHONY: clean lambroll/deploy lambroll/logs lambroll/delete terraform/init terraform/plan terraform/apply terraform/destroy

bootstrap: ../../cmd/cflog2otel/*.go ../../*.go ../../go.*
	GOARCH=arm64 GOOS=linux go build -o bootstrap ../../cmd/cflog2otel/main.go

clean:
	rm -f bootstrap
	rm -f *_dummy.zip

lambroll/deploy: bootstrap
	lambroll --log-level debug deploy
	$(MAKE) clean

lambroll/logs:
	lambroll logs --follow --format=short

lambroll/delete:
	lambroll delete --log-level debug

.PHONY: terraform/init
terraform/init:
	terraform providers lock \
		-platform=darwin_amd64 \
		-platform=darwin_arm64 \
		-platform=linux_amd64 \
		-platform=linux_arm64 \
		-platform=windows_amd64
	terraform init


.PHONY: terraform/plan
terraform/plan:
	terraform plan


.PHONY: terraform/apply
terraform/apply:
	terraform apply
	$(MAKE) clean

.PHONY: terraform/destory
terraform/destroy:
	terraform destroy
