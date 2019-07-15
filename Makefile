stage-all: clean stage-deploy
test-all: clean test-deploy
prod-all: clean prod-deploy

build:
	@echo '--- Building get-new-faces-feeds function ---'
	GOOS=linux go build lambda-get-new-faces/get_new_faces.go
	@echo '--- Building lmm-feeds function ---'
	GOOS=linux go build lambda-lmm/lmm.go
	@echo '--- Building lmhis-feeds function ---'
	GOOS=linux go build lambda-lmhis/lmhis.go
	@echo '--- Building chat-feeds function ---'
	GOOS=linux go build lambda-get-chat/chat.go
	@echo '--- Building discover-feeds function ---'
	GOOS=linux go build discover-function/discover.go

zip_lambda: build
	@echo '--- Zip get-new-faces-feeds function ---'
	zip get_new_faces.zip ./get_new_faces
	@echo '--- Zip lmm-feeds function ---'
	zip lmm.zip ./lmm
	@echo '--- Zip lmhis-feeds function ---'
	zip lmhis.zip ./lmhis
	@echo '--- Zip chat-feeds function ---'
	zip chat.zip ./chat
	@echo '--- Zip discover-feeds function ---'
	zip discover.zip ./discover

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
	rm -rf lmm.zip
	rm -rf lmm
	rm -rf lmhis
	rm -rf lmhis.zip
	rm -rf chat.zip
	rm -rf chat
	rm -rf discover
	rm -rf discover.zip


