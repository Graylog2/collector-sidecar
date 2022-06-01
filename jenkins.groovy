pipeline
{
   agent none

   options
   {
      buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '10', daysToKeepStr: '30', numToKeepStr: '10')
      timestamps()
      withAWS(region:'eu-west-1', credentials:'aws-key-releases')
   }

   tools
   {
     go 'Go'
   }

   environment
   {
     GOPATH = '/home/jenkins/go'
     GO15VENDOREXPERIMENT=1
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
             sh 'go version'
             sh 'go mod vendor'
             sh "make test"
             sh 'make build-all'
             stash name: 'build artifacts', includes: 'build/**'
          }

          post
          {
            cleanup
            {
              cleanWs()
            }
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
            unstash 'build artifacts'
            sh 'make package-all'

            stash name: 'package artifacts', includes: 'dist/pkg/**'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
               cleanWs()
            }

            cleanup
            {
              cleanWs()
            }
         }
      }
   }
}
