stage-all: clean stage-deploy
test-all: clean test-deploy
prod-all: clean prod-deploy

build:
	go get -u github.com/ringoid/commons
	@echo '--- Building get-new-faces-feeds function ---'
	GOOS=linux go build lambda-get-new-faces/get_new_faces.go
	@echo '--- Building warmup-image function ---'
	GOOS=linux go build lambda-warmup/warm_up.go
	@echo '--- Building llm-feeds function ---'
	GOOS=linux go build lambda-lmm/lmm.go

zip_lambda: build
	@echo '--- Zip get-new-faces-feeds function ---'
	zip get_new_faces.zip ./get_new_faces
	@echo '--- Zip warmup-image function ---'
	zip warmup-image.zip ./warm_up
	@echo '--- Zip llm-image function ---'
	zip lmm.zip ./lmm

test-deploy: zip_lambda
	@echo '--- Build lambda test ---'
	@echo 'Package template'
	sam package --template-file feeds-template.yaml --s3-bucket ringoid-cloudformation-template --output-template-file feeds-template-packaged.yaml
	@echo 'Deploy test-feeds-stack'
	sam deploy --template-file feeds-template-packaged.yaml --s3-bucket ringoid-cloudformation-template --stack-name test-feeds-stack --capabilities CAPABILITY_IAM --parameter-overrides Env=test --no-fail-on-empty-changeset

stage-deploy: zip_lambda
	@echo '--- Build lambda stage ---'
	@echo 'Package template'
	sam package --template-file feeds-template.yaml --s3-bucket ringoid-cloudformation-template --output-template-file feeds-template-packaged.yaml
	@echo 'Deploy stage-feeds-stack'
	sam deploy --template-file feeds-template-packaged.yaml --s3-bucket ringoid-cloudformation-template --stack-name stage-feeds-stack --capabilities CAPABILITY_IAM --parameter-overrides Env=stage --no-fail-on-empty-changeset

prod-deploy: zip_lambda
	@echo '--- Build lambda prod ---'
	@echo 'Package template'
	sam package --template-file feeds-template.yaml --s3-bucket ringoid-cloudformation-template --output-template-file feeds-template-packaged.yaml
	@echo 'Deploy prod-feeds-stack'
	sam deploy --template-file feeds-template-packaged.yaml --s3-bucket ringoid-cloudformation-template --stack-name prod-feeds-stack --capabilities CAPABILITY_IAM --parameter-overrides Env=prod --no-fail-on-empty-changeset

clean:
	@echo '--- Delete old artifacts ---'
	rm -rf get_new_faces
	rm -rf get_new_faces.zip
	rm -rf warmup-image.zip
	rm -rf warm_up
	rm -rf lmm.zip
	rm -rf lmm

