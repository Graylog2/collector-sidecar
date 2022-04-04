pipeline
{
   agent none

   options
   {
      buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '10', daysToKeepStr: '30', numToKeepStr: '10')
      timestamps()
      withAWS(region:'eu-west-1', credentials:'aws-key-releases')
      skipDefaultCheckout(true)
   }

   tools
   {
     go 'Go'
   }

   environment
   {
     GOPATH = '/home/jenkins/go'
     GO15VENDOREXPERIMENT=1
     SIDECAR_BRANCH = env.CHANGE_ID ? "${CHANGE_BRANCH}" : "${BRANCH_NAME}"
   }

    stages
            {
                stage('Build')
                        {
                            agent
          {
            label 'linux'
          }
          steps
          {
             checkout([$class: 'GitSCM', branches: [[name: "*/${SIDECAR_BRANCH}"]], extensions: [[$class: 'WipeWorkspace']], userRemoteConfigs: [[url: 'https://github.com/Graylog2/collector-sidecar.git']]])

             sh 'go version'
             sh 'go mod vendor'
             sh "make test"
             sh 'make build-all'
             stash name: 'build artifacts', includes: 'build/**'
          }
       }

      stage('Package')
      {
         agent
         {
           docker
           {
             label 'linux'
             image 'torch/jenkins-fpm-cook:latest'
             args '-u jenkins:docker'
           }
         }

         steps
         {
            checkout([$class: 'GitSCM', branches: [[name: "*/${SIDECAR_BRANCH}"]], extensions: [[$class: 'WipeWorkspace']], userRemoteConfigs: [[url: 'https://github.com/Graylog2/collector-sidecar.git']]])
            unstash 'build artifacts'
            sh 'make package-all'

            stash name: 'package artifacts', includes: 'dist/pkg/**'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
            }
         }
      }
   }
}
