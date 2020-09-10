.PHONY: dep build docker release install test backup

build:
	go build

run: build
	./example

docker:
	docker build -t ${G_SERVICE_NAME} .

run-docker: docker
	docker run --rm -e G_KEY -e G_PROJECT_ID -e PORT=8000 -p 8000:8000 ${G_SERVICE_NAME}

gbuild:
	gcloud builds submit -t gcr.io/${G_PROJECT_ID}/${G_SERVICE_NAME}

deploy: gbuild
	gcloud run deploy ${G_SERVICE_NAME} \
            --region us-central1 \
            --image gcr.io/${G_PROJECT_ID}/${G_SERVICE_NAME} \
            --platform managed \
            --allow-unauthenticated
