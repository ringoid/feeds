# Feeds Service


### STAGE API ENDPOINT IS ``mshn6rkpfb.execute-api.eu-west-1.amazonaws.com``
### PROD API ENDPOINT IS ````

### Get new faces

* url ``https://{API ENDPOINT}/Prod/get_new_faces?accessToken={ACCESS TOKEN}&resolution=480x640``

GET request

Allowed Sizes:

* 480x640
* 720x960
* 1080x1440
* 1440x1920

Headers:

* x-ringoid-android-buildnum : 1       //int, x-ringoid-ios-buildnum in case of iOS
* Content-Type : application/json

 Response Body:
 
    {
        "errorCode":"",
        "errorMessage":"",
        "profiles":[
            {"photoId":"12dd","photoUri":"https://bla-bla.com/sss.jpg"},
            {"photoId":"12ff","photoUri":"https://bla-bla.com/ddd.jpg"}
        ]
    }
    
Possible errorCodes:

* InternalServerError
* WrongRequestParamsClientError
* InvalidAccessTokenClientError
* TooOldAppVersionClientError
