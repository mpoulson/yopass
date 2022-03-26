FROM golang:buster as app
RUN mkdir -p /yopass
WORKDIR /yopass
COPY . .
RUN go build ./cmd/yopass && go build ./cmd/yopass-server

FROM node:16 as website
COPY website /website
WORKDIR /website
#ENV REACT_APP_FALLBACK_LANGUAGE=en
#ENV APPLICATIONINSIGHTSKEY="65a9a116-b453-4feb-8b8c-58efedd18626"

RUN yarn install && yarn build

FROM gcr.io/distroless/base
COPY --from=app /yopass/yopass /yopass/yopass-server /
COPY --from=website /website/build /public
ENTRYPOINT ["/yopass-server"]
