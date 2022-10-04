# Google Cloud Platform

## Requirements

This quick start assumes you have administrator access to your Google Cloud Platform instance. 

## Setting Up your VM  

If this is the first time you’re creating a VM on this project, you will need to enable the `Compute Engine API`. Click `enable` and wait for the service to finish connecting.

This will pull up the `Create an Instance` page - name the instance something relevant to this project, following your naming convention of choice. In addition, select a region and zone that is relevant / close to your physical location. For this instance, we will use the base suggested image. 

<img src="../images/quickstart/google-cloud-platform/gcp1.png">

Under `Identity and API Access`, and click `Allow full access to all Cloud APIs`. Scroll down to `Firewall` and click `Allow HTTPS traffic` and `Allow HTTP traffic`.

<img src="../images/quickstart/google-cloud-platform/gcp2.png">

Scroll down to the bottom and click `Create` to create and deploy the VM.

Congrats you’ve created your VM instance!

## SSH-ing into the VM

On the Compute Engine Dashboard, click on the VM for this project. Under `Details`, click `SSH` and select `Open in browser window`.

<img src="../images/quickstart/google-cloud-platform/gcp3.png">

This will give you a terminal to maneuver, manipulate the VM, and install Coder.

## Install Coder

In the terminal, run the following command 

```sh
curl -fsSL https://coder.com/install.sh | sh  
```

## Run Coder

For this tutorial, we will run Coder as a System service. You can run Coder in [a multitude of different ways](https://coder.com/docs/coder-oss/latest/install).

First, edit the `coder.env` file to enable `CODER_TUNNEL` by setting the value to true with the following command:

``` sh
sudo vim /etc/coder.d/coder.env
``` 

<img src="../images/quickstart/google-cloud-platform/gcp4.png">

Exit vim and run the following command to start Coder as a system service:

```sh
sudo systemctl enable --now coder
``` 

The following command will get you information about the Coder service that is running and is also where the access URL for this Coder instance is written. 

```sh
journalctl -u coder.service -b 
``` 

This will return a series of logs from launching Coder, however, embedded in the launch is the URL for accessing Coder. 

<img src="../images/quickstart/google-cloud-platform/gcp5.png">

In this instance, Coder can be accessed at the URL  `https://fcca2f3bfc9d2e3bf1b9feb50e723448.pit-1.try.coder.app`. 

Copy the URL and run the following command to create the workspace admin:

```sh
coder login <url***.try.coder.app>
```

Fill out the prompts and be sure to save use email and password. These are your admin username and password. 

You can now access Coder on your local machine by navigating to the `***.try.coder.app` URL and logging in with the username and password. 

## Creating and Uploading your First Template

First, run `coder template init` to create your first template. You’ll be given a list of possible templates to potentially use. This tutorial will show you how to create a Linux based template on GCP. 

<img src="../images/quickstart/google-cloud-platform/gcp6.png">

Select the `Develop in Linux on Google Cloud`, then `cd ./gcp-linux` into the folder created from initializing a template. 

Run the following command: 

```sh
coder templates create
```

It will ask for your `project-id`, which you can find on the home page of your GCP Dashboard. 

Given it’s your first time setting up Coder, it may an error that will look like the following:

<img src="../images/quickstart/google-cloud-platform/gcp7.png">

In the error message will be a link. In this case, the URL is `https://console.developes.google.com/apis/api/iam.googles.com/overview:?project=1073148106645`. Copy the respective URL from your error message, and visit it via your browser. It may ask you to enable `Identity and Access Management (IAM) API`. 

Click `enable` and wait as the API initializes for your account. 

Once initialized, click create credentials in the upper right-hand corner. Select the `Compute Engine API` from the dropdown, and select `Application Data` under `What data will you be accessing?`. In addition, select `Yes, I’m using one or more` under `Are you planning on using this API with Compute Engine, Kubernetes Engine, App Engine, or Cloud Functions?`.

<img src="../images/quickstart/google-cloud-platform/gcp8.png">

Back in your GCP terminal, run the `coder templates create` one more time. 

Congrats! You can now create new Linux-based workspaces that use Google Cloud Platform. 

## Next Steps

- [Learn more about template configuration](../templates.md)
- [Configure more IDEs](../ides/web-ides.md)
